package main

import (
	"bytes"
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
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout bytes.Buffer
	stderr bytes.Buffer
	mon    net.Conn
	numcommands int
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

	// wait until VM is up
	time.Sleep(4 * time.Second)
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
	log.Printf("Monitor response:\n%s", response)
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
	Q.stdin.Write([]byte(cmd + "\n"))
	log.Printf("shell: '%s'", cmd)
	Q.stdout.Reset()
	time.Sleep(100 * time.Millisecond) // wait a bit (hack...)
	out := Q.stdout.String()
	n := strings.Index(out, "\n")
	log.Printf("Output:\n%s", out[n+1:])
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
	log.Printf("Saving state...")
	Q.Monitor("delvm 1")
	Q.Monitor("savevm") // no params -> assigns ID 1 (+ tag vm-XXXX)
	log.Printf("...saved.")
}

func (Q *QEmu) Restore() {
	log.Printf("Restoring state...")
	Q.Monitor("loadvm 1")
	log.Printf("...restored.")
}

func CreateProblemIso() {
	// create dir 'current' (if it doesn't exist)
	err := os.MkdirAll("current", 0700)
	if err != nil {
		log.Printf("Cannot create dir 'current'")
	}

	// link problem
	err = os.Remove("current/problem")
	if err != nil {
		log.Printf("Cannot remote 'current/problem': %s", err)
	}
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
		"-uid", "5000",      // garzon user = 5000 (tc = 1001)
		"-o", "shared.iso",
		"current")

	output, err := geniso.CombinedOutput()
	if err != nil {
		log.Printf("genisoimage error: %s", err)
		log.Printf("genisoimage output: %s", output)
	}
}

var qemu = new(QEmu)

func eval() {
	qemu.Restore()
	CreateProblemIso()
	qemu.Monitor("change ide1-cd0 shared.iso") // insert CD-ROM in the VM
	qemu.Shell("mount /dev/cdrom /mnt/cdrom")
	qemu.Shell("su garzon")
	qemu.Shell("ls -la /mnt/cdrom/problem")
	qemu.Shell("exit")
	qemu.Shell("umount /mnt/cdrom")
	qemu.Monitor("eject ide1-cd0")
}

func main() {
	qemu.Start()

	qemu.Save()

	for i := 0; i < 3; i++ {
		eval()
	}

	// wait for input
	// fmt.Printf("Press Enter:")
	// var s string
	// fmt.Scanf("%s", &s)

	qemu.Quit()
}
