package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"os/exec"
	"time"
)

func main() {
	var (
		err    error
		stderr bytes.Buffer
	)

	// 1. Execute QEMU
	cmd := exec.Command("kvm",
		"-kernel", "vmlinuz",
		"-initrd", "initrd.gz",
		"-append", `"tce=vda kmap=qwerty/es vga=788 nodhcp"`,
		"-drive", "file=tce.img,if=virtio",
		"-drive", "file=shared.img,if=virtio",
		"-serial", "stdio",
		"-serial", "mon:unix:monitor,server", // QEMU will wait...
		"-net", "none")

	cmd.Stderr = &stderr
	log.Printf("Starting QEMU.")

	err = cmd.Start()
	if err != nil {
		log.Fatalf("Error executing QEMU: %s", err)
	}

	time.Sleep(100 * time.Millisecond)

	log.Printf("Connecting to monitor...")
	mon, err := net.Dial("unix", "monitor")
	if err != nil {
		fmt.Printf("err: %s", stderr.String())
		log.Fatalf("Cannot connect to QEMU: %s", err)
	}
	log.Printf("... connected.")
	mon.Write([]byte{0x01, 0x63}) // send "ctrl+a c"

	time.Sleep(4 * time.Second)

	log.Printf("Ending QEMU")
	mon.Write([]byte("quit\n"))
	mon.Close()

	log.Printf("Waiting for QEMU to finish...")
	err = cmd.Wait()
	if err != nil {
		fmt.Printf("err: %s", stderr.String())
		log.Fatalf("Wait: %s", err)
	}
	log.Printf("... bye!")
}
