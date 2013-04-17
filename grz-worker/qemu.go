package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type QEmu struct {
	Image  string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	fresh  bool
	// logfile *os.File
}

var magicPrompt string

func init() {
	h := sha1.New()
	fmt.Fprintf(h, "garzon\n")
	magicPrompt = fmt.Sprintf("%x", h.Sum(nil))
	// log.Printf("MagicPrompt: '%s'\n", magicPrompt)
}

func (Q *QEmu) Log(format string, a ...interface{}) {
	log.Printf(format, a...)
}

func (Q *QEmu) args(addargs ...string) (args []string) {
	root := os.Getenv("GARZON_VMS")
	args = []string{
		"-kernel", root + "/vmlinuz",
		"-initrd", root + "/initrd.gz",
		"-append", fmt.Sprintf(`tce=vda nodhcp grz=%s`, magicPrompt),
		"-drive", "file=" + root + "/" + Q.Image + ",if=virtio",
		"-net", "none",
		"-nographic", // implies "-serial stdio -monitor stdio"
	}
	args = append(args, addargs...)
	return
}

func NewVM(image string) (Q *QEmu, err error) {
	root := os.Getenv("GARZON_VMS")
	_, err = os.Stat(filepath.Join(root, image))
	if err != nil {
		return nil, fmt.Errorf("Cannot find image '%s'", image)
	}
	/*
	file, err := os.Create("qemu.log")
	if err != nil {
		log.Fatalf("Cannot open 'qemu.log'")
	}
	 */
	Q = &QEmu{
		Image:   image,
		// logfile: file,
	}
	return
}

func (Q *QEmu) Prepare() {
	Q.Start()
	Q.Shell("stty -echo")                       // suppress echo
	Q.Shell("export PATH=$PATH:/usr/local/bin") // add /usr/local/bin to PATH
	Q.Save()
}

func (Q *QEmu) Start() error {
	Q.cmd = exec.Command("kvm", Q.args()...)
	return Q.start(false)
}

func (Q *QEmu) LoadVM(snapshot string) error {
	Q.Log(`LoadVM("%s")`, snapshot)
	Q.cmd = exec.Command("kvm", Q.args("-loadvm", snapshot)...)
	return Q.start(true)
}

func (Q *QEmu) start(forceNewPrompt bool) error {
	var err error

	Q.Log("Starting QEMU...")

	// Wire std{in,out}
	Q.stdin, err = Q.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("Cannot connect to QEmu's stdin: %s", err)
	}
	Q.stdout, err = Q.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("Cannot connect to QEmu's stdout: %s", err)
	}

	// Launch QEmu
	err = Q.cmd.Start()
	if err != nil {
		return fmt.Errorf("Error executing QEMU: %s", err)
	}
	if forceNewPrompt {
		Q.emit("")
	}
	Q.waitForPrompt(magicPrompt, nil)
	Q.Log("... ready!")
	Q.fresh = true
	return nil
}

var buf = make([]byte, 10000)

func (Q *QEmu) waitForPrompt(prompt string, report func(string)) (output string) {
	pos := 0
	for {
		n, err := Q.stdout.Read(buf)
		if n > 10000 {
			panic("Buffer overflow!")
		}
		output += string(buf[:n])
		// Q.logfile.Write(buf[:n])
		newpos := strings.Index(output[pos:], "\n")
		if newpos != -1 {
			if report != nil {
				report(output[pos : pos+newpos])
			}
			pos += newpos + 1
		}
		if err != io.EOF && err != nil {
			Q.Log("Monitor: read error: %s", err)
			break
		}
		if strings.HasSuffix(output, prompt) {
			// Q.Log("> `%s`", output)
			break
		}
	}
	output = output[:len(output)-len(prompt)]
	return
}

func (Q *QEmu) emit(cmd string) {
	Q.fresh = false
	fmt.Fprintf(Q.stdin, "%s\n", cmd)
	// fmt.Fprintf(Q.logfile, "%s\n", cmd)
}

func (Q *QEmu) emitCtrlA_C() {
	Q.fresh = false
	Q.stdin.Write([]byte{0x01, 0x63}) // emit "ctrl+a c"
	// Q.logfile.Write([]byte{0x01, 0x63})
}

func (Q *QEmu) Monitor(cmd string) {
	Q.fresh = false

	// Activate monitor
	Q.emitCtrlA_C()
	Q.waitForPrompt("(qemu) ", nil)

	// Execute command
	Q.Log("Monitor: '%s'", cmd)
	Q.emit(cmd)
	Q.waitForPrompt("(qemu) ", nil)

	// Activate console
	Q.emitCtrlA_C()
	Q.waitForPrompt("\n", nil) // eat up '\n' produced by QEmu
}

func (Q *QEmu) Shell(cmd string) string {
	return Q.shell(cmd, nil)
}

func (Q *QEmu) ShellLog(cmd string) string {
	Q.Log("Shell: '%s'", cmd)
	output := Q.Shell(cmd)
	Q.Log("Output:\n%s", output)
	return output
}

func (Q *QEmu) ShellReport(cmd string, report func(string)) string {
	return Q.shell(cmd, report)
}

func (Q *QEmu) shell(cmd string, report func(string)) (output string) {
	Q.fresh = false
	Q.emit(cmd)
	return Q.waitForPrompt(magicPrompt, report)
}

func (Q *QEmu) Quit() {
	Q.Log("Ending QEMU")

	Q.emitCtrlA_C()
	Q.waitForPrompt("(qemu) ", nil)
	Q.emit("quit")

	Q.Log("Waiting for QEMU to finish...")
	err := Q.cmd.Wait()
	if err != nil {
		log.Fatalf("Wait: %s", err)
	}
	Q.Log("... bye!")
}

func (Q *QEmu) Kill() {
	Q.cmd.Process.Kill()
}

const SNAPSHOT_NAME = "grz"

func (Q *QEmu) Save() {
	Q.Monitor("delvm " + SNAPSHOT_NAME)
	Q.Monitor("savevm " + SNAPSHOT_NAME)
}

func (Q *QEmu) Reset() {
	if !Q.fresh {
		Q.Monitor("loadvm " + SNAPSHOT_NAME)
		Q.fresh = true
		Q.emit("")
		Q.waitForPrompt(magicPrompt, nil)
	}
}

const limit = 512 // experimental limit size of shell command (?)

func (Q *QEmu) CopyToVM(vmfile, hostfile string) error {
	Q.Log(`CopyToVM("%s", "%s")`, vmfile, hostfile)
	f, err := os.Open(hostfile)
	if err != nil {
		return fmt.Errorf("QEmu.CopyTo: Cannot open infile: %s", err)
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return fmt.Errorf("QEmu.CopyTo: Cannot ReadAll: %s", err)
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	Q.Shell(fmt.Sprintf(`rm %s.base64`, vmfile))
	// Cut the string in pieces due to the limits in shell commands (?)
	for i := 0; i < len(b64); i = i + limit {
		j := i + limit
		if j > len(b64) {
			j = len(b64)
		}
		cmd := fmt.Sprintf(`echo -n "%s" >> %s.base64`, b64[i:j], vmfile)
		Q.Shell(cmd)
	}
	Q.Shell(fmt.Sprintf("ls -l %s.base64", vmfile))
	Q.Shell(fmt.Sprintf("base64 -d %s.base64 > %s", vmfile, vmfile))
	return nil
}

func (Q *QEmu) CopyToHost(hostfile, vmfile string) error {
	Q.Log(`CopyToHost("%s", "%s")`, hostfile, vmfile)
	b64 := Q.Shell(fmt.Sprintf(`base64 %s`, vmfile))
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("QEmu.CopyToHost: Cannot DecodeString: %s", err)
	}
	err = ioutil.WriteFile(hostfile, data, 0600)
	if err != nil {
		return fmt.Errorf("QEmu.CopyToHost: Cannot WriteFile: %s", err)
	}
	return nil
}
