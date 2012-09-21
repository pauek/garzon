package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

type QEmu struct {
	Image  string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

var graphic = flag.Bool("graphic", false, "Show QEmu graphic mode")

var magicPrompt string

func init() {
	h := sha1.New()
	fmt.Fprintf(h, "garzon\n")
	magicPrompt = fmt.Sprintf("%x", h.Sum(nil))
	log.Printf("MagicPrompt: '%s'\n", magicPrompt)
}

func (Q *QEmu) args(addargs ...string) (args []string) {
	root := os.Getenv("GARZON_VMS")
	args = []string{
		"-kernel", root + "/vmlinuz",
		"-initrd", root + "/initrd.gz",
		"-drive", "file=" + root + "/" + Q.Image + ",if=virtio",
		"-append", `"tce=vda kmap=qwerty/es vga=788 nodhcp"`,
		"-net", "none",
		"-nographic",     // implies "-serial stdio -monitor stdio"
	}
	args = append(args, addargs...)
	return
}

func NewVM(image string) *QEmu {
	return &QEmu{Image: image}
}

func (Q *QEmu) Start() {
	log.Printf("Starting QEMU...")
	Q.cmd = exec.Command("kvm", Q.args()...)
	Q.start()
	Q.waitForPrompt(magicPrompt, nil)
	log.Printf("... ready!")
}

func (Q *QEmu) LoadVM() {
	log.Printf("Starting QEMU...")
	Q.cmd = exec.Command("kvm", Q.args("-loadvm", "1")...)
	Q.start()
	Q.emit("") // force new prompt
	Q.waitForPrompt(magicPrompt, nil)
	log.Printf("... ready!")
}

func (Q *QEmu) start() {
	var err error

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
	Q.stdin.Write([]byte{0x01, 0x63}) // emit "ctrl+a c"
}

func (Q *QEmu) emit(cmd string) {
	fmt.Fprintf(Q.stdin, "%s\n", cmd)
}

func (Q *QEmu) Monitor(cmd string) {
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

func (Q *QEmu) Shell(cmd string) {
	Q.shell(cmd, false)
}

func (Q *QEmu) ShellLog(cmd string) {
	Q.shell(cmd, true)
}

func (Q *QEmu) shell(cmd string, showOutput bool) {
	log.Printf("shell: '%s'", cmd)
	Q.emit(cmd)
	output := Q.waitForPrompt(magicPrompt, func(line string) {
		if showOutput {
			fmt.Printf("%s\n", line)
		}
	})
	if showOutput && false {
		log.Printf("Output:\n%s", output[len(cmd)+2:])
	}
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

func (Q *QEmu) Restore() {
	Q.Monitor("loadvm 1")
}
