package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"text/template"

	_ "github.com/mattn/go-sqlite3"
)

var (
	addr = flag.Bool("addr", false, "find open address and print to final-port.txt")
)

// Page is a structure containing title and body
type Page struct {
	Title string
	Body  []byte
}

func (p *Page) save() error {
	db, err := sql.Open("sqlite3", "wiki.db")
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare("INSERT OR REPLACE INTO wiki VALUES((SELECT id FROM wiki WHERE title = ?), ?, ?)")
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(p.Title, p.Title, p.Body)
	if err != nil {
		return err
	}

	tx.Commit()
	return nil
}

func loadPage(title string) (*Page, error) {
	db, err := sql.Open("sqlite3", "wiki.db")
	if err != nil {
		return nil, err
	}

	stmt, err := db.Prepare("SELECT body FROM wiki WHERE title = ?")
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var body []byte

	err = stmt.QueryRow(title).Scan(&body)
	if err != nil {
		return nil, err
	}

	return &Page{Title: title, Body: body}, nil
}

var templates = template.Must(template.ParseGlob("tmpl/*"))

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var linkSyntax = regexp.MustCompile(`\[([a-zA-Z0-9]+)\]`)
var linkTmpl = "<a href=\"/view/%s\">%s</a>"

func viewHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		http.Redirect(w, r, "/edit/"+title, http.StatusFound)
		return
	}

	// Replace all linkSyntax with anchor tag
	p.Body = linkSyntax.ReplaceAllFunc(p.Body, func(s []byte) []byte {
		out := string(s[1 : len(s)-1])
		return []byte(fmt.Sprintf(linkTmpl, out, out))
	})
	renderTemplate(w, "view", p)
}

func editHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		p = &Page{Title: title}
	}
	renderTemplate(w, "edit", p)
}

func saveHandler(w http.ResponseWriter, r *http.Request, title string) {
	body := r.FormValue("body")
	p := &Page{Title: title, Body: []byte(body)}
	err := p.save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/"+title, http.StatusFound)
}

var validPath = regexp.MustCompile("^/(|edit|save|view)/([a-zA-Z0-9]+)$")

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			fn(w, r, "TestPage")
			return
		}
		fn(w, r, m[2])
	}
}

func initDb() {
	if _, err := os.Stat("./wiki.db"); err != nil {
		log.Println("db does not exists, create one")
		db, err := sql.Open("sqlite3", "wiki.db")
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()

		stmt := `CREATE TABLE wiki(id integer not null primary key, title text, body blob);`
		if _, err = db.Exec(stmt); err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	flag.Parse()
	initDb()
	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/edit/", makeHandler(editHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))
	http.HandleFunc("/", makeHandler(viewHandler))

	if *addr {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("final-port.txt", []byte(l.Addr().String()), 0644)
		if err != nil {
			log.Fatal(err)
		}
		s := &http.Server{}
		s.Serve(l)
		return
	}

	http.ListenAndServe(":8080", nil)
}
