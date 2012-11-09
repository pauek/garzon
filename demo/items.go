package main

import (
	"bytes"
	"fmt"
	"regexp"
	T "html/template"
	"os"
	"path/filepath"
	"strings"
)

type Item struct {
	Title, Root, Path string
	Items             []*Item
}

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

var nums = regexp.MustCompile(`^[0-9]+\. `)
func (I Item) TitleNoNums() string {
	return nums.ReplaceAllLiteralString(I.Title, "")
}

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

const group = `<h2>{{.TitleNoNums}}</h2>
<ul>{{range .Items}}
   <li>{{.Html}}</li>{{end}}
</ul>
`
const item = `<a href="/p/{{.Path}}">{{.TitleNoNums}}</a>`

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
