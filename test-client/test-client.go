package main

import (
	"code.google.com/p/go.net/websocket"
	"flag"
	"fmt"
	gsrv "garzon/server"
	"io"
	"log"
	"os"
	"strings"
)

func main() {
	grzServer := os.Getenv("GARZON_SERVER")
	if grzServer == "" {
		grzServer = "localhost:7070"
	}

	origin := "http://" + grzServer + "/"
	url := "ws://" + grzServer + "/submit"

	flag.Parse()
	if len(flag.Args()) < 1 {
		fmt.Println("usage: test-client <ProblemID> <Data>")
	}
	subm := gsrv.Submission{flag.Arg(0), flag.Arg(1)}

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
			fmt.Println(update)
			break
		} else if err != nil {
			log.Printf("Error receiving: %s", err)
		}
		fmt.Printf("\r%s\r%s", strings.Repeat(" ", 80), update)
	}
	ws.Close()
}