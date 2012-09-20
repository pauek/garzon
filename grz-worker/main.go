package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
)

func createProblemIso(filename string) {
	// create dir 'current' (if it doesn't exist)
	err := os.MkdirAll("current", 0700)
	if err != nil {
		log.Printf("Cannot create dir 'current'")
	}

	// link problem
	err = os.Remove("current/problem")
	if err != nil {
		log.Printf("Cannot remove 'current/problem': %s", err)
	}
	err = os.Symlink(
		"/home/pauek/Academio/Problems/Say yes/",
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
		"-o", filename,
		"current")

	output, err := geniso.CombinedOutput()
	if err != nil {
		log.Printf("genisoimage error: %s", err)
		log.Printf("genisoimage output: %s", output)
	}
}

var qemu *QEmu

func eval() {
	qemu.Restore()
	createProblemIso("shared.iso")
	qemu.Monitor("change ide1-cd0 shared.iso") // insert CD-ROM in the VM
	qemu.Shell("mount /dev/cdrom /mnt/cdrom")
	qemu.Shell("su garzon")

	// Ejecutar el judge...
	qemu.ShellLog("ls -la /mnt/cdrom/problem")

	qemu.Shell("exit")
	qemu.Shell("umount /mnt/cdrom")
	qemu.Monitor("eject ide1-cd0")
	err := os.Remove("shared.iso")
	if err != nil {
		log.Printf("Cannot remove 'shared.iso'")
	}
}

var image string
var prepare bool

func main() {
	flag.StringVar(&image, "image", "gcc.qcow2", "Specify image file to use")
	flag.BoolVar(&prepare, "prepare", false, "Only create the snapshot")
	flag.Parse()

	qemu = NewVM(image)


	if prepare {
		qemu.Start()
		qemu.Save()
		qemu.Quit()
	} else {
		qemu.LoadVM()
		qemu.ShellLog("cat /etc/fstab")
		// eval()
		qemu.Quit()
	}
}
