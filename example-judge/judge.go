package main

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	gsrv "garzon/server"
	"log"
	"net/http"
)

func newSubmission(ws *websocket.Conn) {
	var subm gsrv.Submission
	err := websocket.JSON.Receive(ws, &subm)
	if err != nil {
		log.Printf("Error receiving job: %s", err)
	}
	veredict, err := gsrv.Judge(subm, func (msg string) {
		websocket.JSON.Send(ws, msg)
	})
	if err != nil {
		veredict = fmt.Sprintf("Error: %s", err)
	} 
	websocket.JSON.Send(ws, veredict)
}

func hRoot(w http.ResponseWriter, req *http.Request) {
	// TODO: List (and link) problems
	fmt.Fprintf(w, "hello, world!")
}

func main() {
	gsrv.Handle()
	http.Handle("/submit", websocket.Handler(newSubmission))
	http.HandleFunc("/", hRoot)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
