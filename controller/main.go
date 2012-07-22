package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"time"
)

type QEmu struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout bytes.Buffer
	stderr bytes.Buffer
	mon    net.Conn
}

func (Q *QEmu) Start() {
	var err error 

	// 1. Execute QEMU
	Q.cmd = exec.Command("kvm",
		"-kernel", "vmlinuz",
		"-initrd", "initrd.gz",
		"-append", `"tce=vda kmap=qwerty/es vga=788 nodhcp"`,
		"-drive", "file=tce.img,if=virtio",
		"-serial", "stdio",
		"-serial", "mon:unix:monitor,server", // QEMU will wait...
		"-net", "none")

	Q.stdin, err = Q.cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Cannot connect to QEmu's stdin: %s", err)
	}
	Q.cmd.Stdout = &Q.stdout
	Q.cmd.Stderr = &Q.stderr
	log.Printf("Starting QEMU.")

	err = Q.cmd.Start()
	if err != nil {
		log.Fatalf("Error executing QEMU: %s", err)
	}
	time.Sleep(100 * time.Millisecond) // IMPORTANT: wait before connecting

	log.Printf("Connecting to monitor...")
	Q.mon, err = net.Dial("unix", "monitor")
	if err != nil {
		fmt.Printf("err: %s", Q.stderr.String())
		log.Fatalf("Cannot connect to QEMU: %s", err)
	}
	log.Printf("... connected.")
	Q.mon.Write([]byte{0x01, 0x63}) // send "ctrl+a c"
}

func (Q *QEmu) Monitor(cmd string) {
	Q.mon.Write([]byte(cmd + "\n"))
	time.Sleep(100 * time.Millisecond) // wait a bit
}

func (Q *QEmu) Shell(cmd string) {
	Q.stdout.Reset()
	
}

func (Q *QEmu) Quit() {
	Q.Monitor("quit")
	Q.mon.Close()

	log.Printf("Waiting for QEMU to finish...")
	err := Q.cmd.Wait()
	if err != nil {
		fmt.Printf("err: %s", Q.stderr.String())
		log.Fatalf("Wait: %s", err)
	}
	log.Printf("... bye!")
}

func CreateProblemIso() {
	// create dir 'current' (if it doesn't exist)
	err := os.MkdirAll("current", 0700)
	if err != nil {
		log.Printf("Cannot create dir 'current'")
	}

	// link problem
	err = os.Symlink(
		"/pub/Academio/Problems/Cpp/ficheros/SumaEnteros.prog/",
		"current/problem",
	)
	if err != nil {
		log.Printf("Cannot create symlink: %s", err)
	}

	// link judge

	// link solution

	// gen iso image
	geniso := exec.Command("genisoimage",
		"-f",                // follow symlinks
		"-file-mode", "400", // read-only for tc
		"-uid", "1001", // tc user = 1001
		"-o", "shared.iso",
		"current")

	output, err := geniso.CombinedOutput()
	if err != nil {
		log.Printf("genisoimage error: %s", err)
		log.Printf("genisoimage output: %s", output)
	}
}

func main() {
	qemu := new(QEmu)
	qemu.Start()

	CreateProblemIso()
	qemu.Monitor("change ide1-cd0 shared.iso") // insert CD-ROM in the VM

	// wait for input
	fmt.Printf("Press Enter:")
	var s string
	fmt.Scanf("%s", &s)

	// Finish
	log.Printf("Ending QEMU")

	qemu.Quit()
}
