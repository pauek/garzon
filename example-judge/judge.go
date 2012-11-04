package main

import (
	"code.google.com/p/go.net/websocket"
	"fmt"
	gsrv "garzon/server"
	T "html/template"
	"log"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
)

type Problem struct {
	ID    string
	Title string
	Dir   string
}

var Problems = make(map[string]Problem)

func subdirs(dirname string) (dirs []string) {
	f, err := os.Open(dirname)
	if err != nil {
		return nil
	}
	list, err := f.Readdir(-1)
	f.Close()
	if err != nil {
		return nil
	}
	for _, d := range list {
		if d.IsDir() && d.Name()[0] != '.' {
			dirs = append(dirs, filepath.Join(dirname, d.Name()))
		}
	}
	return
}

func ProblemCatalog(path string) {
	for _, root := range filepath.SplitList(path) {
		for _, subdir := range subdirs(root) {
			for _, problem := range subdirs(subdir) {
				ID, err := filepath.Rel(root, problem)
				if err != nil {
					log.Fatalf("Boum! %s", err)
				}
				Problems[ID] = Problem{ID, filepath.Base(problem), problem}
			}
		}
	}
}

func init() {
	path := &gsrv.ProblemPath
	*path = os.Getenv("GARZON_PATH")
	if *path == "" {
		*path = "."
	}
	ProblemCatalog(*path)
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

const index = `
<!doctype html>
<html>
  <head>
    <title>Example Judge</title>
  </head>
  <body>
    <h1>Problems</h1>
    <ol>{{range .}}
       <li><a href="/p/{{.ID}}">{{.Title}}</a></li>{{end}}
    </ol>
  </body>
</html>
`

const problem = `
<!doctype html>
<html>
<head>
   <title>Problem: {{.problem.Title}}</title>
   <script src="//ajax.googleapis.com/ajax/libs/jquery/1.8.2/jquery.min.js"></script>
   <link rel="stylesheet" href="/js/codemirror.css">
   <script src="/js/codemirror.js"></script>
   <script src="/js/clike.js"></script>
   <style>
     .CodeMirror { 
        border: 1px solid #abb; 
        font-family: "Source Code Pro"; 
        font-size: 12;
     }
   </style>
</head>
<body>
  <h1>{{.problem.Title}}</h1>
  {{.doc}}
  <h2>Submit</h2>
  <textarea id="code"></textarea><br />
  <button>Submit</button>
  <div id="status"></div>
<script>

function submit() {
   var host = document.location.host;
   ws = new WebSocket("ws://" + host + "/submit")
   ws.onopen  = function () { 
      console.log("Connected!");
      ws.send(JSON.stringify({
         ProblemID: "{{.problem.ID}}", 
         Data: editor.getValue(),
      }));
   }
   ws.onclose = function () { 
      console.log("Disconnected!"); 
   }
   ws.onmessage = function (e) {
      var msg = JSON.parse(e.data);
      $("#status").html("<pre>" + msg + "</pre>");
   }
}

var editor;

$(document).ready(function () {
   $("button").click(submit);
   editor = CodeMirror.fromTextArea(document.getElementById("code"), {
      lineNumbers: true,
      matchBrackets: true,
      mode: "text/x-c++src"
   });
})
</script>
</body>
</html>
`

var tIndex = T.Must(T.New("index").Parse(index))
var tProblem = T.Must(T.New("problem").Parse(problem))

func hRoot(w http.ResponseWriter, req *http.Request) {
	err := tIndex.Execute(w, Problems)
	if err != nil {
		fmt.Println("ERROR", err)
	}
}

func hProblem(w http.ResponseWriter, req *http.Request) {
	id := req.URL.Path[len("/p/"):]
	prob, ok := Problems[id]
	if !ok {
		http.Error(w, "Not Found", http.StatusNotFound)
	}
	var doc string
	docfilename := filepath.Join(prob.Dir, "doc.html")
	if docfile, err := os.Open(docfilename); err == nil {
		if _doc, err := ioutil.ReadAll(docfile); err == nil {
			doc = string(_doc)
		}
		docfile.Close()
	}
	err := tProblem.Execute(w, map[string]interface{}{
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
