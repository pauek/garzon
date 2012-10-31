package main

import (
	"code.google.com/p/go.net/websocket"
	gsrv "garzon/server"
	"fmt"
	"io"
	"log"
	"strings"
)

func main() {
	origin := "http://localhost/"
	url := "ws://localhost:8080/submit"
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		log.Fatal(err)
	}

	submission := gsrv.Submission{"/home/pauek/Academio/Problems/Test/42", []byte("43")}
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
		fmt.Printf("\r%s\r%s", strings.Repeat(" ", 80), update)
	}
}