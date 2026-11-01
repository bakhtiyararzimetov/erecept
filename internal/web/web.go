// Package web: страницы врача и аптеки (html/template + htmx).
package web

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed templates/*.html static/*
var assets embed.FS

type Web struct {
	DB    *pgxpool.Pool
	pages map[string]*template.Template
}

// New разбирает шаблоны один раз: каждая страница = layout.html + <страница>.html.
func New(pool *pgxpool.Pool) *Web {
	w := &Web{DB: pool, pages: map[string]*template.Template{}}
	files, err := fs.Glob(assets, "templates/*.html")
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		name := f[len("templates/") : len(f)-len(".html")]
		if name == "layout" {
			continue
		}
		t, err := template.ParseFS(assets, "templates/layout.html", f)
		if err != nil {
			log.Fatalf("шаблон %s: %v", f, err)
		}
		w.pages[name] = t
	}
	return w
}

func (w *Web) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(w.loadUser)

	static, _ := fs.Sub(assets, "static")
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServerFS(static)))

	r.Get("/", w.home)
	r.Get("/login", w.loginForm)
	r.Post("/login", w.loginSubmit)
	r.Post("/logout", w.logout)

	r.With(requireRole("doctor")).Get("/doctor", w.doctorHome)
	r.With(requireRole("pharmacy")).Get("/pharmacy", w.pharmacyHome)
	return r
}

// render выводит layout.html + <page>.html. В data всегда есть User.
func (w *Web) render(rw http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	t, ok := w.pages[page]
	if !ok {
		log.Printf("нет шаблона %q", page)
		http.Error(rw, "Страница не найдена", http.StatusInternalServerError)
		return
	}
	if data == nil {
		data = map[string]any{}
	}
	data["User"] = currentUser(r)
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", data); err != nil {
		log.Printf("шаблон %s: %v", page, err)
		http.Error(rw, "Не удалось показать страницу, попробуйте ещё раз", http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Write(buf.Bytes())
}

// Временные страницы: A3 и A5 заменяют их настоящими.
func (w *Web) doctorHome(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "placeholder", map[string]any{"Title": "Мои рецепты", "Text": "Здесь будет форма нового рецепта и список ваших рецептов."})
}

func (w *Web) pharmacyHome(rw http.ResponseWriter, r *http.Request) {
	w.render(rw, r, "placeholder", map[string]any{"Title": "Мои лекарства", "Text": "Здесь будет кабинет аптеки: цены и наличие лекарств."})
}
