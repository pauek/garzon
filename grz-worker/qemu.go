package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"io"
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
}

var graphic = flag.Bool("graphic", false, "Show QEmu graphic mode")

var magicPrompt string

func init() {
	h := sha1.New()
	fmt.Fprintf(h, "garzon\n")
	magicPrompt = fmt.Sprintf("%x", h.Sum(nil))
	// log.Printf("MagicPrompt: '%s'\n", magicPrompt)
}

func (Q *QEmu) args(addargs ...string) (args []string) {
	root := os.Getenv("GARZON_VMS")
	args = []string{
		"-kernel", root + "/vmlinuz",
		"-initrd", root + "/initrd.gz",
		"-drive", "file=" + root + "/" + Q.Image + ",if=virtio",
		"-drive", "file=" + Tmp("shared.img") + ",if=virtio",
		"-append", fmt.Sprintf(`tce=vda kmap=qwerty/es vga=788 nodhcp grz=%s`, magicPrompt),
		"-net", "none",
		"-snapshot",      // Write to temp files instead of image
		"-nographic",     // implies "-serial stdio -monitor stdio"
	}
	args = append(args, addargs...)
	return
}

func NewVM(image string) *QEmu {
	root := os.Getenv("GARZON_VMS")
	_, err := os.Stat(filepath.Join(root, image))
	if err != nil {
		log.Fatalf("Cannot find image '%s'", image)
	}
	return &QEmu{Image: image}
}

func (Q *QEmu) Start() {
	Q.cmd = exec.Command("kvm", Q.args()...)
	Q.start(false)
}

func (Q *QEmu) LoadVM() {
	Q.cmd = exec.Command("kvm", Q.args("-loadvm", "1")...)
	Q.start(true)
}

func (Q *QEmu) start(forceNewPrompt bool) {
	var err error

	log.Printf("Starting QEMU...")

	// Wire std{in,out}
	Q.stdin, err = Q.cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Cannot connect to QEmu's stdin: %s", err)
	}
	Q.stdout, err = Q.cmd.StdoutPipe()
	if err != nil {
		log.Fatalf("Cannot connect to QEmu's stdout: %s", err)
	}

	// Launch QEmu
	err = Q.cmd.Start()
	if err != nil {
		log.Fatalf("Error executing QEMU: %s", err)
	}

	if forceNewPrompt {
		Q.emit("")
	}
	Q.waitForPrompt(magicPrompt, nil)
	Q.shell("stty -echo", nil) // suppress echo

	log.Printf("... ready!")
	Q.fresh = true
}

var buf = make([]byte, 10000)

func (Q *QEmu) waitForPrompt(prompt string, report func(string)) (output string) {
	pos := 0
	for {
		n, err := Q.stdout.Read(buf)
		output += string(buf[:n])
		if report != nil {
			newpos := strings.Index(output[pos:], "\n")
			if newpos != -1 {
				report(output[pos:pos+newpos])
				pos += newpos+1
			}
		}
		if err != io.EOF && err != nil {
			log.Printf("Monitor: read error: %s", err)
			break
		}
		if strings.HasSuffix(output, prompt) {
			break
		}
	}
	output = output[:len(output)-len(prompt)]
	return
}

func (Q *QEmu) emitCtrlA_C() {
	Q.fresh = false
	Q.stdin.Write([]byte{0x01, 0x63}) // emit "ctrl+a c"
}

func (Q *QEmu) emit(cmd string) {
	Q.fresh = false
	fmt.Fprintf(Q.stdin, "%s\n", cmd)
}

func (Q *QEmu) Monitor(cmd string) {
	Q.fresh = false

	// Activate monitor
	Q.emitCtrlA_C()
	Q.waitForPrompt("(qemu) ", nil)

	// Execute command
	log.Printf("Monitor: '%s'", cmd)
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
	log.Printf("Shell: '%s'", cmd)
	output := Q.shell(cmd, nil)
	log.Printf("Output:\n%s", output)
	return output
}

func (Q *QEmu) ShellReport(cmd string, report func(string)) string {
	return Q.shell(cmd, report)
}

func (Q *QEmu) shell(cmd string, report func(string)) (output string) {
	Q.fresh = false
	Q.emit(cmd)
	output = Q.waitForPrompt(magicPrompt, report)
	return
}

func (Q *QEmu) Quit() {
	log.Printf("Ending QEMU")

	Q.emitCtrlA_C()
	Q.waitForPrompt("(qemu) ", nil)
	Q.emit("quit")

	log.Printf("Waiting for QEMU to finish...")
	err := Q.cmd.Wait()
	if err != nil {
		log.Fatalf("Wait: %s", err)
	}
	log.Printf("... bye!")
}

func (Q *QEmu) Save() {
	Q.Monitor("delvm 1")
	Q.Monitor("savevm") // no params -> assigns ID 1 (+ tag vm-XXXX)
}

func (Q *QEmu) Reset() {
	if !Q.fresh {
		Q.Monitor("loadvm 1")
		Q.fresh = true
	}
}
