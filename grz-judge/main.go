package main

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	gsrv "garzon/server"
	T "html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

var courses Item

func ReadCourses(path string) {
	for _, root := range filepath.SplitList(path) {
		item := &Item{Title: "", Root: root, Path: ""}
		item.Read()
		courses.Items = append(courses.Items, item)
	}
}

func init() {
	path := &gsrv.ProblemPath
	*path = os.Getenv("GARZON_PATH")
	if *path == "" {
		*path = "."
	}
	ReadCourses(*path)
	gsrv.Handle()
}

func newSubmission(ws *websocket.Conn) {
	var subm gsrv.Submission
	err := websocket.JSON.Receive(ws, &subm)
	if err != nil {
		log.Printf("Error receiving job: %s", err)
	}
	veredict, err := gsrv.Judge(subm, func(msg string) {
		websocket.JSON.Send(ws, msg)
	})
	if err != nil {
		veredict = fmt.Sprintf("Error: %s", err)
	}
	websocket.JSON.Send(ws, veredict)
}

var tmpl = T.Must(T.ParseFiles("templates.html"))

func hRoot(w http.ResponseWriter, req *http.Request) {
	err := tmpl.ExecuteTemplate(w, "index", courses)
	if err != nil {
		fmt.Println("ERROR", err)
	}
}

func hProblem(w http.ResponseWriter, req *http.Request) {
	id := req.URL.Path[len("/p/"):]
	var prob *Item
	for _, course := range courses.Items {
		if prob = course.Find(id); prob != nil {
			break
		}
	}
	if prob == nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	var doc string
	docfilename := filepath.Join(prob.Dir(), "doc.html")
	if docfile, err := os.Open(docfilename); err == nil {
		if _doc, err := ioutil.ReadAll(docfile); err == nil {
			doc = string(_doc)
		}
		docfile.Close()
	}
	err := tmpl.ExecuteTemplate(w, "problem", map[string]interface{}{
		"doc":     T.HTML(doc),
		"problem": prob,
	})
	if err != nil {
		fmt.Println("ERROR", err)
	}
}

func main() {
	http.Handle("/submit", websocket.Handler(newSubmission))
	http.Handle("/js/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/", hRoot)
	http.HandleFunc("/p/", hProblem)
	log.Fatal(http.ListenAndServe(":7070", nil))
}
