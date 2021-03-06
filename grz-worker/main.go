package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	gsrv "garzon/server"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"code.google.com/p/go.net/websocket"
)

var (
	tempdir string
	homedir string
	qemu    *QEmu
)

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

func CreateTempDir() {
	for i := 0; i < 100; i++ {
		if TryTempDir(i) {
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
		log.Printf("Cannot remove temporary directory '%s': %s", tempdir, err)
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
	err := qemu.CopyToGuest("/tmp/"+base, judgesrc)
	if err != nil {
		return fmt.Errorf("Cannot copy '%s' to guest: %s", judgesrc, err)
	}
	qemu.Shell("export GOROOT=/mnt/vda/go")
	qemu.Shell("export PATH=$PATH:/mnt/vda/go/bin")

	// Compile
	var cmd string

	switch ext {
	case ".go":
		cmd = fmt.Sprintf("go build -o /tmp/judge.bin /tmp/%s", base)

	case ".c", ".cc", ".cpp", ".cxx":
		cc := "g++"
		if ext == ".c" {
			cc = "gcc"
		}
		cmd = fmt.Sprintf("%s -o /tmp/judge.bin /tmp/%s", cc, base)

	default:
		return fmt.Errorf("Language not supported")
	}

	if output := qemu.Shell(cmd); output != "" {
		return fmt.Errorf("Judge does not compile:\n%s", output)
	}

	// Get the binary from VM
	err = qemu.CopyToHost(judgebin, "/tmp/judge.bin")
	if err != nil {
		return fmt.Errorf("Cannot copy VM file to '%s': %s", judgebin, err)
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
		if !strings.HasSuffix(f, "~") { // do not consider backup files
			candidates = append(candidates, f)
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

	// Look for already compiled judge, otherwise compile it
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

func Eval(problemDir string, solution []byte, report func(msg string)) (veredict string, err error) {
	CreateCurrentDir()
	defer RemoveCurrentDir()

	report("Preparing...")

	if err := CreateISO(problemDir, solution); err != nil {
		return "", err
	}
	qemu.Reset()
	qemu.Monitor("change ide1-cd0 " + filepath.Join(tempdir, "iso"))

	var (
		nlin       int
		hash       string
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
			line = line[:len(line)-1] // strip '\r'
			if isVeredict {
				veredict += line + "\n" // FIXME: \r por aquí?
			} else {
				log.Printf("report: '%s'\n", line)
				report(line)
			}
		}
	}) // execute judge
	if veredict == "" {
		veredict = "Judge Failed"
	}
	log.Printf("Veredict: %s", veredict)
	qemu.Monitor("eject ide1-cd0")
	RemoveISO()
	return
}

var (
	image   string
	prepare bool
)

func ensureTempDir(tmpdir string) {
	err := os.RemoveAll(tmpdir)
	if err != nil {
		log.Fatalf("Cannot remove tmp dir '%s': %s", tmpdir, err)
	}
	err = os.Mkdir(tmpdir, 0700)
	if err != nil {
		log.Fatalf("Cannot create tmp dir '%s': %s", tmpdir, err)
	}
}

func CatchTermination() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		RemoveTempDir()
		qemu.Kill()
		os.Exit(0)
	}()
}

func Serve() {
	var (
		err                                     error
		msg, veredict, uncompressDir, targzFile string
		ws                                      *websocket.Conn
		file                                    *os.File
	)

	qemu, err = NewVM(image)
	if err != nil {
		log.Fatalf("Cannot create VM: %s", err)
	}
	qemu.StartAndReset()
	defer qemu.Quit()

	grzServer := os.Getenv("GARZON_SERVER")
	if grzServer == "" {
		grzServer = "localhost:7070"
	}

	for {
		// Connect Loop
		origin := fmt.Sprintf("http://%s/", grzServer)
		url := fmt.Sprintf("ws://%s/_new_worker", grzServer)
		for ws, err = websocket.Dial(url, "", origin); err != nil; {
			log.Printf("Error dialing: %s", err)
			time.Sleep(5 * time.Second)
			log.Printf("Retrying...")
		}
		log.Printf("Connected!")

		for {
			// Receive job
			var submission gsrv.Submission
			err = websocket.JSON.Receive(ws, &submission)
			if err != nil {
				log.Printf("Cannot receive job: %s", err)
				break
			}
			id := submission.ProblemID
			data := submission.Data

			// Reply "ok" || "send problem" || "alive"
			//   TODO: Check cache for ProblemID
			if id == "" {
				websocket.JSON.Send(ws, "alive")
				continue
			}
			log.Printf("Received job '%s': %d bytes", id, len(data))
			log.Printf("Data:\n%s", data)

			websocket.JSON.Send(ws, "need targz")
			var problem struct {
				Id    string
				Targz []byte
			}
			err = websocket.JSON.Receive(ws, &problem)
			if err != nil {
				msg = "Error receiving tar.gz"
				goto fail
			}
			log.Printf("Received problem: %d bytes", len(problem.Targz))

			// Save problem + uncompress
			targzFile = Tmp("problem.tar.gz")
			file, err = os.Create(targzFile)
			if err != nil {
				msg = "Error saving problem.tar.gz"
				goto fail
			}
			_, err = file.Write(problem.Targz)
			if err != nil {
				msg = "Cannot write tar.gz"
				goto fail
			}
			uncompressDir = Tmp("garzon")
			ensureTempDir(uncompressDir)
			err = exec.Command("tar", "-xzf", targzFile, "-C", uncompressDir).Run()
			if err != nil {
				msg = fmt.Sprintf("Cannot uncompress '%s'", targzFile)
				goto fail
			}
			log.Printf("Uncompressed '%s'", targzFile)

			// Eval
			veredict, err = Eval(uncompressDir, data, func(update string) {
				websocket.JSON.Send(ws, update)
			})
			if err != nil {
				msg = "Eval error"
				goto fail
			}
			log.Printf("VEREDICT: %s", veredict)
			websocket.JSON.Send(ws, "VEREDICT\n"+veredict)
			continue

		fail:
			log.Printf("%s: %s", msg, err)
			websocket.JSON.Send(ws, fmt.Sprintf("ERROR: %s: %s ", msg, err))
		}

		// Close connection
		ws.Close()
	}
}

func Prepare() {
	var err error
	qemu, err = NewVM(image)
	if err != nil {
		log.Fatalf("Cannot create VM: %s", err)
	}
	qemu.Prepare()
	qemu.Quit()
}

func Test1() {
	var err error
	qemu, err = NewVM(image)
	if err != nil {
		log.Fatalf("Cannot create VM: %s", err)
	}
	qemu.StartAndReset()
	err = qemu.CopyToGuest("/tmp/test", "/home/pauek/rnd")
	if err != nil {
		log.Printf("ERROR: Cannot copy to vm: %s", err)
	}
	qemu.ShellLog("ls -l /tmp/")
	qemu.ShellLog("md5sum /tmp/test")

	err = qemu.CopyToHost("/home/pauek/fromvm", "/bin/busybox")
	if err != nil {
		log.Printf("ERROR: Cannot copy to host: %s", err)
	}
	qemu.ShellLog("md5sum /bin/busybox")
	qemu.Quit()
}

func main() {
	flag.StringVar(&image, "image", "garzon.qcow2", "Specify image file to use")
	flag.BoolVar(&prepare, "prepare", false, "Only create the snapshot")
	flag.Parse()

	EnsureHomeDir()
	CreateTempDir()
	defer RemoveTempDir()
	CatchTermination()

	if prepare {
		Prepare()
	} else {
		// Test1()
		Serve()
	}
}
