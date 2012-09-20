package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

type QEmu struct {
	Image     string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    bytes.Buffer
	stderr    bytes.Buffer
	mon       net.Conn
}

var graphic = flag.Bool("graphic", false, "Show QEmu graphic mode")

func (Q *QEmu) args(addargs ...string) (args []string) {
	root := os.Getenv("GARZON_VMS")
	args = []string{
		"-kernel", root + "/vmlinuz",
		"-initrd", root + "/initrd.gz",
		"-drive", "file=" + root + "/" + Q.Image + ",if=virtio",
		"-append", `"tce=vda kmap=qwerty/es vga=788 nodhcp"`,
		"-serial", "stdio",
		"-serial", "mon:unix:monitor,server", // QEMU will wait...
		"-net", "none",
	}
	args = append(args, addargs...)
	if !*graphic {
		args = append(args, "-nographic")
	}
	return
}

func NewVM(image string) *QEmu {
	return &QEmu{Image: image}
}

func (Q *QEmu) Start() {
	Q.cmd = exec.Command("kvm", Q.args()...)
	Q.start()
	time.Sleep(10 * time.Second) // wait until VM is up
	log.Printf("Ready.")
}

func (Q *QEmu) LoadVM() {
	Q.cmd = exec.Command("kvm", Q.args("-loadvm", "1")...)
	Q.start()
	time.Sleep(3 * time.Second) // empirical
	log.Printf("Ready.")
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
	time.Sleep(500 * time.Millisecond) // IMPORTANT: wait before connecting

	// Connect to monitor
	Q.mon, err = net.Dial("unix", "monitor")
	if err != nil {
		fmt.Printf("err: %s", Q.stderr.String())
		log.Fatalf("Cannot connect to QEMU: %s", err)
	}
	Q.mon.Write([]byte{0x01, 0x63}) // send "ctrl+a c"

	Q.waitMonitorPrompt(true) // first wait
	log.Printf("Connected to monitor.")
}

var buf = make([]byte, 1000)

func (Q *QEmu) waitMonitorPrompt(first bool) {
	Q.mon.SetDeadline(time.Now().Add(10 * time.Second))
	var response string
	for {
		n, _ := Q.mon.Read(buf)
		response += string(buf[:n])
		if strings.HasSuffix(response, "(qemu) ") {
			break
		}
	}
	if response != "(qemu) " && !first {
		fmt.Printf("%s", response[:len(response)-7])
	}
}

func (Q *QEmu) Monitor(cmd string) {
	log.Printf("Monitor: '%s'", cmd)
	Q.mon.Write([]byte(cmd + "\n")) // emit command
	Q.waitMonitorPrompt(false)
}

func (Q *QEmu) Shell(cmd string) {
	Q.shell(cmd, false, 300 * time.Millisecond)
}

func (Q *QEmu) ShellLog(cmd string) {
	Q.shell(cmd, true, 300 * time.Millisecond)
}

func (Q *QEmu) ShellWait(cmd string, dur time.Duration) {
	Q.shell(cmd, true, dur)
}

func (Q *QEmu) shell(cmd string, showOutput bool, dur time.Duration) {
	Q.stdin.Write([]byte(cmd + "\n"))
	log.Printf("shell: '%s'", cmd)
	Q.stdout.Reset()
	time.Sleep(dur) // wait a bit (hack...)
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

	// Erase file 'monitor'
	err := os.Remove("monitor")
	if err != nil {
		log.Printf("Cannot remove 'monitor': %s", err)
	}

	log.Printf("Waiting for QEMU to finish...")
	err = Q.cmd.Wait()
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
