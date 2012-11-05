package main

import (
	"bytes"
	"code.google.com/p/go.net/websocket"
	"fmt"
	gsrv "garzon/server"
	T "html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type Item struct {
	Title, Root, Path string
	Items []*Item
}
var courses Item

func NewItem(absdir, root string) (I *Item) {
	I = &Item{}
	I.Title = filepath.Base(absdir)
	I.Root = root
	path, err := filepath.Rel(root, absdir)
	if err != nil {
		panic(err)
	}
	I.Path = path
	return
}

func (I *Item) Dir() string { return filepath.Join(I.Root, I.Path) }

func (I *Item) IsGroup() bool { return len(I.Items) > 0 }

func (I *Item) Find(path string) *Item {
	if path == "" {
		return I
	}
	base, rest := path, ""
	if i := strings.Index(path, "/"); i != -1 {
		base, rest = path[:i], path[i+1:]
	}
	for _, item := range I.Items {
		if base == item.Title {
			return item.Find(rest)
		}
	}
	return nil
}

func (I *Item) Read() {
	if subdirs := subdirs(I.Dir()); len(subdirs) > 0 {
		I.Items = make([]*Item, len(subdirs))
		for i := range I.Items {
			I.Items[i] = NewItem(subdirs[i], I.Root)
			I.Items[i].Read()
		}
	}
}

const group = `<h2>{{.Title}}</h2>
<ul>{{range .Items}}
   <li>{{.Html}}</li>{{end}}
</ul>
`
const item = `<a href="/p/{{.Path}}">{{.Title}}</a>`

var tGroup = T.Must(T.New("group").Parse(group))
var tItem = T.Must(T.New("group").Parse(item))

func (I Item) Html() T.HTML {
	var err error
	var b bytes.Buffer
	if I.Title == "" {
		for _, item := range I.Items {
			fmt.Fprint(&b, item.Html())
		}
	} else if I.IsGroup() {
		err = tGroup.Execute(&b, I)
	} else {
		err = tItem.Execute(&b, I)
	}
	if err != nil {
		fmt.Fprintf(&b, "ERROR: %s", err)
	}
	return T.HTML(b.String())
}

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
		if d.IsDir() && !startsWithAny(d.Name(), "._") {
			dirs = append(dirs, filepath.Join(dirname, d.Name()))
		}
	}
	return
}

func startsWithAny(s, chars string) bool {
	return len(s) > 0 && strings.Index(chars, s[:1]) != -1
}

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

const index = `
<!doctype html>
<html>
  <head>
    <title>Example Judge</title>
    <style> 
      ul { list-style-type: none; } 
    </style>
  </head>
  <body>
    <h1>Problems</h1>
    {{.Html}}
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
      pre { margin-left: 2em; background: #ddd; padding: .5em 1em; }
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
         ProblemID: "{{.problem.Path}}", 
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
	err := tIndex.Execute(w, courses)
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
