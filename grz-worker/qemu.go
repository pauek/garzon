package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type QEmu struct {
	Image  string
	Root   string
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

func (Q *QEmu) Filename(which string) (filename string) { 
	filename = Q.Root + "/"
	switch which {
	case "kernel": filename += "vmlinuz"
	case "initrd": filename += "initrd.gz"
	case "image":  filename += Q.Image
	case "io":     filename += Q.Image + ".io"
	}
	return 
}


var iofile string

func (Q *QEmu) args(addargs ...string) (args []string) {
	args = []string{
		"-kernel", Q.Filename("kernel"), 
		"-initrd", Q.Filename("initrd"),
		"-append", fmt.Sprintf(`tce=vda nodhcp grz=%s`, magicPrompt),
		"-drive", fmt.Sprintf(`file=%s,if=virtio`, Q.Filename("image")),
		"-net", "none",
		"-usb",
		// IO using a virtio serial port
		"-device", "virtio-serial",
		"-chardev", fmt.Sprintf(`socket,path=%s,server,nowait,id=io`, Q.Filename("io")),
		"-device", "virtserialport,chardev=io,name=io.0",
		// 
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
	Q = &QEmu{
		Image:   image,
		Root:    root,
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

func (Q *QEmu) StartAndReset() error {
	Q.Log(`LoadVM("%s")`, SNAPSHOT_NAME)
	Q.cmd = exec.Command("kvm", Q.args("-loadvm", SNAPSHOT_NAME)...)
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

func (Q *QEmu) Monitor(cmd string) (output string) {
	Q.fresh = false

	// Activate monitor
	Q.emitCtrlA_C()
	Q.waitForPrompt("(qemu) ", nil)

	// Execute command
	Q.Log("Monitor: '%s'", cmd)
	Q.emit(cmd)
	output = Q.waitForPrompt("(qemu) ", nil)

	// Activate console
	Q.emitCtrlA_C()
	Q.waitForPrompt("\n", nil) // eat up '\n' produced by QEmu
	return
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

func (Q *QEmu) CopyToGuest(vmfile, hostfile string) error {
	Q.Log(`CopyToVM("%s", "%s")`, vmfile, hostfile)

	// 1. Prepare goroutine that copies file to socket
	var goerr error
	go func() {
		fin, err := os.Open(hostfile)
		if err != nil {
			goerr = err
			return
		}
		conn, err := net.Dial("unix", Q.Filename("io"))
		if err != nil {
			goerr = err
			return
		}
		bytes, err := io.Copy(conn, fin)
		if err != nil {
			goerr = err
			return
		}
		Q.Log(`Sent %d bytes`, bytes)
		goerr = conn.Close()
	}()

	// 2. Copy the file
	output := Q.Shell(fmt.Sprintf(`cat /dev/virtio-ports/io.0 > %s`, vmfile))
	if output != "" {
		return fmt.Errorf("QEmu.CopyToGuest: cat command returned something: %s", output)
	}
	if goerr != nil {
		return fmt.Errorf("QEmu.CopyToGuest: copy to socket failed: %s", goerr)
	}
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
