package server

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

var numWorkers int32 = 0

type Submission struct {
	ProblemID string
	Data      []byte
}

type Job struct {
	Submission
	updates chan string
}

var jobs = make(chan *Job)

func sendProblem(ws *websocket.Conn, job *Job) error {
	// TODO
	return nil
}

func handleJob(ws *websocket.Conn, job *Job) error {
	// Send to worker
	websocket.JSON.Send(ws, job.Submission)
	var reply string
	if err := websocket.JSON.Receive(ws, &reply); err != nil {
		return err
	}
	switch reply {
	case "alive":
		return nil
	case "send problem":
		sendProblem(ws, job)
		return nil
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
	return handleJob(ws, &Job{Submission{"", []byte{}}, nil})
}

func newWorker(ws *websocket.Conn) {
	atomic.AddInt32(&numWorkers, 1)
	log.Printf("Connected [%s] (active = %d)\n", ws.RemoteAddr(), numWorkers)
	for {
		select {
		case j := <-jobs:
			if err := handleJob(ws, j); err != nil {
				log.Printf("Error handling job: %s", err)
				jobs <- j
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

func Submit(subm Submission, report func(msg string)) (veredict string, err error) {
	if numWorkers == 0 {
		return "ERROR", fmt.Errorf("No workers")
	}
	newjob := Job{subm, make(chan string)}
	select {
	case jobs <- &newjob:
		var s string
		for s = range newjob.updates {
			if !strings.HasPrefix(s, "VEREDICT\n") {
				report(s)
			}
		}
		veredict = s[len("VEREDICT\n"):]
	case <-time.After(5 * time.Second):
		return "ERROR", fmt.Errorf("No worker responding")
	}
	return veredict, nil
}

func Handle(path string) {
	http.Handle(path, websocket.Handler(newWorker))
}