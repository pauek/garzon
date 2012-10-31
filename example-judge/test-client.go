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

	Submissions := []gsrv.Submission{
		{"Test/BlahBlah", []byte("x")}, // no problem Test/43
		{"Test/42", []byte("43")},      // Reject
		{"Test/42", []byte("42")},      // Accept
	}

	for _, subm := range Submissions {
		ws, err := websocket.Dial(url, "", origin)
		if err != nil {
			log.Fatal(err)
		}
		if err := websocket.JSON.Send(ws, subm); err != nil {
			log.Fatalf("Cannot send: %s", err)
		}
		for {
			var update string
			err := websocket.JSON.Receive(ws, &update)
			if err == io.EOF {
				break
			} else if (strings.HasPrefix(update, "ERROR")) {
				log.Print(update)
				break
			} else if err != nil {
				log.Printf("Error receiving: %s", err)
			}
			fmt.Printf("\r%s\r%s", strings.Repeat(" ", 80), update)
		}
		ws.Close()
	}
}