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
			log.Printf("Temporary directory: '%s'", tempdir)
			return
		}
	}
}

func Tmp(filename string) string {
	return filepath.Join(tempdir, filename)
}

func CreateCurrentDir() {
	if err := os.Mkdir(Tmp("current"), 0700); err != nil {
		log.Printf("Cannot create '%s': %s", Tmp("current"), err)
	}
}

func RemoveCurrentDir() {
	if err := os.RemoveAll(Tmp("current")); err != nil {
		log.Printf("Cannot remove '%s': %s", Tmp("current"), err)
	}
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

func CreateISO(problemDir string, solution []byte) error {
	// Check Problem dir
	if info, err := os.Stat(problemDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("'%s' is not a directory", problemDir)
		}
	} else {
		return fmt.Errorf("'%s' does not exist", problemDir)
	}
	LinkProblem(problemDir)
	LinkJudge(problemDir)
	if err := AddSolution(solution); err != nil {
		return err
	}

	// gen iso image
	geniso := exec.Command("genisoimage",
		"-f", // follow symlinks
		// "-file-mode", "400", // read-only for tc
		"-uid", "5000", // garzon user = 5000 (tc = 1001)
		"-o", filepath.Join(tempdir, "iso"),
		Tmp("current"))

	output, err := geniso.CombinedOutput()
	if err != nil {
		log.Printf("genisoimage error: %s", err)
		log.Printf("genisoimage output: %s", output)
	}
	return nil
}

func RemoveISO() {
	err := os.Remove(filepath.Join(tempdir, "iso"))
	if err != nil {
		log.Printf("Cannot remove 'iso'")
	}
}

func Eval(problemDir string, solution []byte) error {
	CreateCurrentDir()

	if err := CreateISO(problemDir, solution); err != nil {
		return err
	}
	qemu.Reset()
	qemu.Monitor("change ide1-cd0 " + filepath.Join(tempdir, "iso"))

	var (
		nlin       int
		hash       string
		veredict   string
		isVeredict bool
	)
	qemu.ShellReport("/mnt/vda/garzon.sh", func(line string) {
		nlin++
		switch {
		case nlin == 1:
			hash = line
		case line == hash:
			isVeredict = true
		default:
			if isVeredict {
				veredict += line[:len(line)-1] + "\n" // FIXME: \r por aquí?
			} else {
				fmt.Printf("%s\n", line)
			}
		}
	}) // execute judge
	fmt.Printf("Veredict: %s", veredict)
	qemu.Monitor("eject ide1-cd0")
	RemoveISO()
	RemoveCurrentDir()

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
	for _, p := range flag.Args() {
		if absp, err := filepath.Abs(p); err == nil {
			if err := Eval(absp, []byte("43")); err != nil {
				log.Printf("Eval error: %s", err)
			}
		} else {
			log.Printf("Error with path '%s'", absp)
		}
	}
	qemu.Quit()
}
