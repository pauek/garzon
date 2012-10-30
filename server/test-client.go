package main

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	"io"
	"log"
)

type Submission struct {
	ProblemID string
	Data []byte
}

func main() {
	origin := "http://localhost/"
	url := "ws://localhost:6060/submit"
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		log.Fatal(err)
	}
	
	submission := Submission{"/home/pauek/Academio/Problems/Test/42", []byte("42")}
	if err := websocket.JSON.Send(ws, submission); err != nil {
		log.Fatalf("Cannot send: %s", err)
	}
	for {
		var update string
		err := websocket.JSON.Receive(ws, &update)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error receiving: %s", err)
		}
		fmt.Printf("                                  \r%s", update)
	}
}