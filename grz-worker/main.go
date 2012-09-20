package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	tempdir string
	current string
)

func CreateTempDir() {
	tmp := os.TempDir()
	for i := 0; i < 100; i++ {
		tempdir = filepath.Join(tmp, fmt.Sprintf("grz-worker-%04d", i))
		_, err := os.Stat(tempdir)
		if err != nil {
			if err := os.Mkdir(tempdir, 0700); err != nil {
				log.Printf("Cannot create '%s': %s", tempdir, err)
				break
			}
			current = filepath.Join(tempdir, "current")
			if err := os.Mkdir(current, 0700); err != nil {
				log.Printf("Cannot create '%s': %s", current, err)
				break
			}
			log.Printf("Temporary directory: '%s'", tempdir)
			return
		}
	}
	log.Fatalf("Cannot create temporary directory")
}

func RemoveTempDir() {
	err := os.RemoveAll(tempdir)
	if err != nil {
		log.Printf("Cannot remove '%s': %s", tempdir, err)
	}
}

func LinkProblem(problemDir string) {
	// link problem
	prob := filepath.Join(current, "problem")
	if _, err := os.Stat(prob); err == nil {
		err := os.Remove(prob)
		if err != nil {
			log.Printf("Cannot remove '%s': %s", prob, err)
		}
	}
	if err := os.Symlink(problemDir, prob); err != nil {
		log.Printf("Cannot create symlink: %s", err)
	}
}

func AddSolution(solution []byte) {
	
}

func LinkJudge() {
}

func CreateISO() {
	// gen iso image
	geniso := exec.Command("genisoimage",
		"-f",                // follow symlinks
		"-file-mode", "400", // read-only for tc
		"-uid", "5000", // garzon user = 5000 (tc = 1001)
		"-o", filepath.Join(tempdir, "iso"),
		current)

	output, err := geniso.CombinedOutput()
	if err != nil {
		log.Printf("genisoimage error: %s", err)
		log.Printf("genisoimage output: %s", output)
	}
}

func RemoveISO() {
	err := os.Remove(filepath.Join(tempdir, "iso"))
	if err != nil {
		log.Printf("Cannot remove 'iso'")
	}
}

func Eval(problemDir string) {
	LinkProblem(problemDir)
	CreateISO()
	qemu.Monitor("change ide1-cd0 " + filepath.Join(tempdir, "iso")) // insert CD-ROM in the VM
	qemu.Shell("mount /dev/cdrom /mnt/cdrom")
	qemu.Shell("su garzon")
	qemu.Shell("cd /mnt/cdrom/problem")

	// Ejecutar el judge...
	qemu.ShellLog("ls -la")

	qemu.Shell("exit")
	qemu.Shell("umount /mnt/cdrom")
	qemu.Monitor("eject ide1-cd0")
	RemoveISO()
	qemu.Restore()
}

var (
	qemu    *QEmu
	image   string
	prepare bool
)

func main() {
	flag.StringVar(&image, "image", "gcc.qcow2", "Specify image file to use")
	flag.BoolVar(&prepare, "prepare", false, "Only create the snapshot")
	flag.Parse()

	CreateTempDir()
	defer RemoveTempDir()

	qemu = NewVM(image)

	if prepare {
		qemu.Start()
		qemu.Save()
		qemu.Quit()
		return
	} 
	
	qemu.LoadVM()
	probs := []string{
		"/pub/Academio/Problems/Cpp/introduccion/SumaEnteros.prog",
		"/pub/Academio/Problems/Cpp/introduccion/CuantosCeros.prog",
	}
	for _, p := range probs {
		Eval(p)
	}
	qemu.Quit()
}
