package server

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"code.google.com/p/go.net/websocket"
)

var numWorkers int32 = 0

var ProblemPath = "."

type Submission struct {
	ProblemID string
	Data      []byte
}

type Problem struct {
	Id    string
	Targz []byte
}

type Job struct {
	Submission
	updates chan string
}

var jobs = make(chan *Job)

func isDir(dir string) bool {
	if info, err := os.Stat(dir); err == nil {
		return info.IsDir()
	}
	return false
}

func findProblem(id string) (dir string) {
	for _, root := range filepath.SplitList(ProblemPath) {
		dir = filepath.Join(root, id)
		if isDir(dir) {
			return
		}
	}
	return ""
}

func compressProblem(dir string) (targz []byte, err error) {
	filename := filepath.Join(os.TempDir(), "problem.tar.gz")
	err = exec.Command("tar", "-czf", filename, "-C", dir, ".").Run()
	if err != nil {
		return nil, fmt.Errorf("Cannot compress: %s", err)
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("Cannot open '%s': %s", filename, err)
	}
	targz, err = ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("Cannot read '%s': %s", filename, err)
	}
	file.Close()
	// TODO: erase 'problem.tar.gz'
	return
}

func handleJob(ws *websocket.Conn, job *Job) error {
	// Find problem
	var dir string
	if job.ProblemID != "" {
		dir = findProblem(job.ProblemID)
		if dir == "" {
			msg := fmt.Sprintf("Problem '%s' not found", job.ProblemID)
			job.updates <- fmt.Sprintf("ERROR: %s", msg)
			close(job.updates)
			return fmt.Errorf("%s", msg)
		}
	}

	// Submit (+ Send tar.gz is necessary)
	websocket.JSON.Send(ws, job.Submission)
	var reply string
	if err := websocket.JSON.Receive(ws, &reply); err != nil {
		return err
	}
	switch reply {
	case "alive":
		return nil

	case "need targz":
		targz, err := compressProblem(dir)
		if err != nil {
			return fmt.Errorf("Cannot send problem: %s", err)
		}
		websocket.JSON.Send(ws, Problem{Id: job.ProblemID, Targz: targz})
		log.Printf(`Sent problem "%s"`, dir)

	case "ok":
	}
	log.Printf(`Submitted: %s`, job.Submission.ProblemID)

	// Wait for updates (& veredict)
	var msg string
	for {
		if err := websocket.JSON.Receive(ws, &msg); err != nil {
			job.updates <- fmt.Sprintf("Error receiving updates: %s", err)
			close(job.updates)
			break
		}

		job.updates <- msg

		if strings.HasPrefix(msg, "VEREDICT") ||
			strings.HasPrefix(msg, "ERROR") {
			close(job.updates)
			break
		}
	}
	return nil
}

func isAlive(ws *websocket.Conn) error {
	return handleJob(ws, &Job{Submission{"", nil}, nil})
}

func newWorker(ws *websocket.Conn) {
	atomic.AddInt32(&numWorkers, 1)
	log.Printf("Connected [%s] (active = %d)\n", ws.RemoteAddr(), numWorkers)
	for {
		select {
		case j := <-jobs:
			if err := handleJob(ws, j); err != nil {
				log.Printf("Error handling job: %s", err)
				// jobs <- j
			}
		case <-time.After(10 * time.Second):
			if err := isAlive(ws); err != nil {
				ws.Close()
				atomic.AddInt32(&numWorkers, -1)
				log.Printf("Worker died (active = %d)", numWorkers)
				return
			}
		}
	}
}

func Judge(subm Submission, report func(msg string)) (veredict string, err error) {
	if numWorkers == 0 {
		return "ERROR", fmt.Errorf("No workers")
	}
	if report != nil {
		report("In queue")
	}
	newjob := Job{subm, make(chan string)}
	select {
	case jobs <- &newjob:
		var s string
		for s = range newjob.updates {
			if !strings.HasPrefix(s, "VEREDICT\n") && report != nil {
				report(s)
			}
		}
		veredict = s[len("VEREDICT\n"):]

	case <-time.After(10 * time.Second):
		return "ERROR", fmt.Errorf("No worker responding: try again later")
	}
	return veredict, nil
}

func Handle() {
	http.Handle("/_new_worker", websocket.Handler(newWorker))
}
