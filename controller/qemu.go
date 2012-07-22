package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"strings"
	"time"
)

type QEmu struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout bytes.Buffer
	stderr bytes.Buffer
	mon    net.Conn
	numcommands int
}

var args = []string{
	"-kernel", "vmlinuz",
	"-initrd", "initrd.gz",
	"-append", `"tce=vda kmap=qwerty/es vga=788 nodhcp"`,
	"-drive", "file=tce.img,if=virtio",
	"-serial", "stdio",
	"-serial", "mon:unix:monitor,server", // QEMU will wait...
	"-net", "none",
}

func (Q *QEmu) Start() {
	Q.cmd = exec.Command("kvm", args...)
	Q.start()
	time.Sleep(4 * time.Second)	// wait until VM is up
}

func (Q *QEmu) LoadVM() {
	Q.cmd = exec.Command("kvm", append(args, "-loadvm", "1")...)
	Q.start()
	time.Sleep(1 * time.Second)
}

func (Q *QEmu) start() {
	var err error

	log.Printf("Starting QEMU.")

	// Wire std{in,out,err}
	Q.stdin, err = Q.cmd.StdinPipe()
	if err != nil {
		log.Fatalf("Cannot connect to QEmu's stdin: %s", err)
	}
	Q.cmd.Stdout = &Q.stdout
	Q.cmd.Stderr = &Q.stderr

	// Launch QEmu
	err = Q.cmd.Start()
	if err != nil {
		log.Fatalf("Error executing QEMU: %s", err)
	}
	time.Sleep(100 * time.Millisecond) // IMPORTANT: wait before connecting

	// Connect to monitor
	Q.mon, err = net.Dial("unix", "monitor")
	if err != nil {
		fmt.Printf("err: %s", Q.stderr.String())
		log.Fatalf("Cannot connect to QEMU: %s", err)
	}
	log.Printf("Connected to monitor.")
	Q.mon.Write([]byte{0x01, 0x63}) // send "ctrl+a c"
}

var buf = make([]byte, 1000)

func (Q *QEmu) waitMonitorPrompt() {
	Q.mon.SetDeadline(time.Now().Add(10 * time.Second))
	var response string
	for {
		n, _ := Q.mon.Read(buf)
		response += string(buf[:n])
		if strings.HasSuffix(response, "(qemu) ") {
			break
		}
	}
	// log.Printf("Monitor response:\n%s", response)
}

func (Q *QEmu) Monitor(cmd string) {
	if Q.numcommands == 0 {
		Q.waitMonitorPrompt()
	}
	Q.numcommands++
	log.Printf("Monitor: '%s'", cmd)
	Q.mon.Write([]byte(cmd + "\n")) // emit command
	Q.waitMonitorPrompt()
}

func (Q *QEmu) Shell(cmd string) {
	Q.shell(cmd, false)
}

func (Q *QEmu) ShellLog(cmd string) {
	Q.shell(cmd, true)
}

func (Q *QEmu) shell(cmd string, showOutput bool) {
	Q.stdin.Write([]byte(cmd + "\n"))
	log.Printf("shell: '%s'", cmd)
	Q.stdout.Reset()
	time.Sleep(100 * time.Millisecond) // wait a bit (hack...)
	out := Q.stdout.String()
	n := strings.Index(out, "\n")
	if showOutput {
		log.Printf("Output:\n%s", out[n+1:])
	}
}

func (Q *QEmu) Quit() {
	log.Printf("Ending QEMU")

	Q.mon.Write([]byte("quit\n")) // don't wait
	Q.mon.Close()

	log.Printf("Waiting for QEMU to finish...")
	err := Q.cmd.Wait()
	if err != nil {
		fmt.Printf("err: %s", Q.stderr.String())
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
