package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	tempdir string
	homedir string
)

const sharedImgSize = 20 // in MB

func EnsureHomeDir() {
	homedir = filepath.Join(os.Getenv("HOME"), ".grz")
	judgesdir := filepath.Join(homedir, "judges")
	if err := os.MkdirAll(judgesdir, 0700); err != nil {
		log.Fatalf("Error creating home dir '%s': %s", judgesdir, err)
	}
}

func TryTempDir(i int) bool {
	tempdir = filepath.Join(os.TempDir(), fmt.Sprintf("grz-worker-%04d", i))
	if _, err := os.Stat(tempdir); err != nil {
		if err := os.Mkdir(tempdir, 0700); err != nil {
			log.Printf("Cannot create '%s': %s", tempdir, err)
			return false
		}
		log.Printf("Temporary directory: '%s'", tempdir)
		return true
	}
	return false
}

func CreateSharedImg() {
	file, err := os.Create(Tmp("shared.img"))
	if err != nil {
		log.Fatalf("Cannot create '%s': %s", Tmp("shared.img"), err)
	}
	for i := 0; i < sharedImgSize; i++ {
		file.Write(make([]byte, 1024*1024)) // 1MB
	}
	file.Close()
}

func CreateTempDir() {
	for i := 0; i < 100; i++ {
		if TryTempDir(i) {
			CreateSharedImg()
			return
		}
	}
	log.Fatalf("Cannot create Temp Dir!")
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

func Sha1Sum(filename string) (sha1sum string, err error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	sha1 := sha1.New()
	io.Copy(sha1, file)
	return fmt.Sprintf("%x", sha1.Sum(nil)), nil
}

func CopyFile(dest, source string, bytes int64) (written int64, err error) {
	from, err := os.Open(source)
	if err != nil {
		return 0, fmt.Errorf("Cannot open '%s': %s", source, err)
	}
	to, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return 0, fmt.Errorf("Cannot create '%s': %s", dest, err)
	}
	if bytes == -1 {
		written, err = io.Copy(to, from)
	} else {
		written, err = io.CopyN(to, from, bytes)
	}
	if err != nil {
		return 0, fmt.Errorf("Cannot CopyFile: %s", err)
	}
	// fmt.Printf("Wrote %d bytes\n", written)
	from.Close()
	to.Close()
	return
}

func CompileJudgeInVM(judgesrc, judgebin string) error {
	log.Printf("Compiling judge: %s", judgesrc)
	base := filepath.Base(judgesrc)
	ext := filepath.Ext(judgesrc)

	// Transfer sources to VM
	written, err := CopyFile(Tmp("shared.img"), judgesrc, -1)
	if err != nil {
		return fmt.Errorf("Cannot copy '%s' to shared image: %s", judgesrc, err)
	}
	qemu.Shell(fmt.Sprintf("dd if=/dev/vdb of=/tmp/%s bs=1 count=%d", base, written))

	// Compile
	switch ext {
	case ".go":
		cmd := strings.Join([]string{
			"/mnt/vda/src/go/bin/go build",
			"-o /tmp/judge.bin",
			fmt.Sprintf("/tmp/%s", base),
		}, " ")
		output := qemu.Shell(cmd)
		if output != "" {
			return fmt.Errorf("Judge does not compile:\n%s", output)
		}

	case ".cc", ".cpp", ".cxx":
		cmd := strings.Join([]string{
			"PATH=$PATH:/usr/local/bin",
			"/usr/local/bin/g++",
			"-o /tmp/judge.bin",
			fmt.Sprintf("/tmp/%s", base),
		}, " ")
		output := qemu.Shell(cmd)
		if output != "" {
			return fmt.Errorf("Judge does not compile:\n%s", output)
		}

	default:
		return fmt.Errorf("Language not supported")
	}

	// Get the binary from VM
	output := qemu.Shell("dd if=/tmp/judge.bin of=/dev/vdb bs=1")
	qemu.Shell("sync")
	var bytes int64
	fmt.Sscanf(output, "%d", &bytes)
	_, err = CopyFile(judgebin, Tmp("shared.img"), bytes)
	if err != nil {
		return fmt.Errorf("Cannot copy shared image to '%s': %s", judgebin, err)
	}

	qemu.Reset()

	return nil
}

func CompileAndLinkJudge(problemDir string) error {
	// Find judge source
	results, err := filepath.Glob(filepath.Join(problemDir, "judge.*"))
	if err != nil {
		return fmt.Errorf("Cannot glob 'judges.*': %s", err)
	}
	candidates := []string{}
	for _, f := range results {
		ext := filepath.Ext(f)
		for _, e := range []string{".go", ".cc", ".cpp", ".cxx"} {
			if ext == e {
				candidates = append(candidates, f)
				break
			}
		}
	}
	if len(candidates) > 1 {
		return fmt.Errorf("Multiple judge source files")
	} else if len(candidates) == 0 {
		return fmt.Errorf("No judge source file")
	}
	judgesrc := candidates[0]

	// compute Sha1sum of judge source code
	sha1, err := Sha1Sum(judgesrc)
	if err != nil {
		log.Printf("Cannot compute sha1sum of '%s': %s", judgesrc, err)
	}

	// Look for already compiled judge otherwise compile it
	judgebin := filepath.Join(homedir, "judges/"+sha1)
	if _, err1 := os.Stat(judgebin); err1 != nil {
		if err2 := CompileJudgeInVM(judgesrc, judgebin); err2 != nil {
			return fmt.Errorf("Cannot compile: %s", err2)
		}
	}

	// Link judge
	if err := os.Symlink(judgebin, Tmp("current/judge")); err != nil {
		return fmt.Errorf("Cannot create symlink: %s", err)
	}

	// Chmod +x
	if err := os.Chmod(judgebin, 0700); err != nil {
		return fmt.Errorf("Cannot make '%s' executable", judgebin)
	}

	return nil
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
	if err := CompileAndLinkJudge(problemDir); err != nil {
		return err
	}
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

	if output, err := geniso.CombinedOutput(); err != nil {
		return fmt.Errorf("genisoimage error: %s\noutput:\n%s", err, output)
	}
	return nil
}

func RemoveISO() error {
	if err := os.Remove(filepath.Join(tempdir, "iso")); err != nil {
		return fmt.Errorf("Cannot remove 'iso': %s", err)
	}
	return nil
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
	qemu.ShellReport("/bin/garzon.sh", func(line string) {
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
	EnsureHomeDir()
	CreateTempDir()
	defer RemoveTempDir()

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
