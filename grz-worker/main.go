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
			for _, subdir := range []string{"current"} {
				dir := filepath.Join(tempdir, subdir)
				if err := os.Mkdir(dir, 0700); err != nil {
					log.Printf("Cannot create '%s': %s", dir, err)
					goto fatal
				}
			}
			log.Printf("Temporary directory: '%s'", tempdir)
			return
		}
	}

fatal:
	log.Fatalf("Cannot create temporary directory")
}

func Tmp(filename string) string {
	return filepath.Join(tempdir, filename)
}

func RemoveTempDir() {
	err := os.RemoveAll(tempdir)
	if err != nil {
		log.Printf("Cannot remove '%s': %s", tempdir, err)
	}
}

func LinkProblem(problemDir string) {
	// link problem
	prob := Tmp("current/problem")
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

func AddSolution(solution []byte) error {
	f, err := os.Create(Tmp("current/solution"))
	if err != nil {
		return fmt.Errorf("Cannot save solution: %s", err)
	}
	f.Write(solution)
	f.Close()
	return nil
}

func LinkJudge(problemDir string) {
	judgesrc := filepath.Join(problemDir, "judge.go")
	if err := os.Symlink(judgesrc, Tmp("current/judge.go")); err != nil {
		log.Printf("Cannot create symlink: %s", err)
	}
}

func CreateISO() {
	// gen iso image
	geniso := exec.Command("genisoimage",
		"-f",                // follow symlinks
   // "-file-mode", "400", // read-only for tc
	   "-uid", "5000", // garzon user = 5000 (tc = 1001)
		"-o", filepath.Join(tempdir, "iso"),
		Tmp("current"))

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


func Eval(problemDir string, solution []byte) error {
	LinkProblem(problemDir)
	LinkJudge(problemDir)
	if err := AddSolution(solution); err != nil {
		return err
	}
	CreateISO()
	qemu.Monitor("change ide1-cd0 " + filepath.Join(tempdir, "iso")) // insert CD-ROM in the VM
	qemu.ShellLog("/mnt/vda/garzon.sh") // execute judge
	qemu.Monitor("eject ide1-cd0")
	RemoveISO()
	qemu.Restore()
	return nil
}

var (
	qemu    *QEmu
	image   string
	prepare bool
)

func main() {
	flag.StringVar(&image, "image", "garzon.qcow2", "Specify image file to use")
	flag.BoolVar(&prepare, "prepare", false, "Only create the snapshot")
	flag.Parse()

	qemu = NewVM(image)

	if prepare {
		qemu.Start()
		qemu.Save()
		qemu.Quit()
		return
	}

	CreateTempDir()
	defer RemoveTempDir()

	qemu.LoadVM()
	probs := []string{
		"/pub/Academio/Problems/Test/42",
	}
	for _, p := range probs {
		if err := Eval(p, []byte("42")); err != nil {
			fmt.Printf("Eval error: %s", err)
		}
	}
	qemu.Quit()
}
