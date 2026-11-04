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
	DB       *pgxpool.Pool
	Dictate  Dictator // голосовое заполнение рецепта; nil — выключено
	PitchKey string   // ключ пульта презентации /pitch; пусто — пульт выключен
	pages    map[string]*template.Template
	live     *pitchLive
}

// New разбирает шаблоны один раз: каждая страница = layout.html + <страница>.html.
func New(pool *pgxpool.Pool) *Web {
	w := &Web{DB: pool, pages: map[string]*template.Template{}, live: newPitchLive()}
	files, err := fs.Glob(assets, "templates/*.html")
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range files {
		name := f[len("templates/") : len(f)-len(".html")]
		if name == "layout" || name == "item_row" || name == "pharmacy_row" {
			continue
		}
		t, err := template.New(name).Funcs(template.FuncMap{"money": money}).ParseFS(assets, "templates/layout.html", "templates/item_row.html", "templates/pharmacy_row.html", f)
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
	// Временно: презентация для жюри (static/pitch), открывается без входа.
	r.Get("/pitch", func(rw http.ResponseWriter, r *http.Request) {
		http.Redirect(rw, r, "/static/pitch/", http.StatusFound)
	})
	r.Get("/pitch/live", w.pitchLiveStream)
	r.Post("/pitch/live", w.pitchLiveSet)

	r.Get("/", w.home)
	r.Get("/login", w.loginForm)
	r.Post("/login", w.loginSubmit)
	r.Post("/logout", w.logout)

	r.Get("/r/{code}", w.publicPage)
	r.Get("/qr/{file}", w.qrPNG)

	r.Route("/doctor", func(r chi.Router) {
		r.Use(requireRole(roleDoctor))
		r.Get("/", w.doctorNew)
		r.Get("/item-row", w.itemRow)
		r.Post("/prescriptions", w.createPrescription)
		r.Post("/dictate", w.dictate)
		r.Get("/prescriptions", w.prescriptionList)
		r.Get("/prescriptions/{id}", w.prescriptionPage)
		r.Get("/prescriptions/{id}/print", w.prescriptionPrint)
	})
	r.Route("/pharmacy", func(r chi.Router) {
		r.Use(requireRole(rolePharmacy))
		r.Get("/", w.pharmacyHome)
		r.Get("/sample.csv", w.pharmacySample)
		r.Post("/items", w.pharmacyAdd)
		r.Post("/items/{id}", w.pharmacyUpdate)
		r.Post("/items/{id}/delete", w.pharmacyDelete)
		r.Post("/csv", w.pharmacyCSV)
	})
	return r
}

// render выводит layout.html + <page>.html. В data всегда есть User.
func (w *Web) render(rw http.ResponseWriter, r *http.Request, page string, data map[string]any) {
	w.renderStatus(rw, r, http.StatusOK, page, data)
}

// renderStatus — как render, но с нужным кодом ответа (например, 422 для формы с ошибками).
func (w *Web) renderStatus(rw http.ResponseWriter, r *http.Request, status int, page string, data map[string]any) {
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
	rw.WriteHeader(status)
	rw.Write(buf.Bytes())
}

// renderPlain выводит страницу без шапки (печать, публичная страница):
// шаблон страницы сам определяет полный HTML и называется как файл.
func (w *Web) renderPlain(rw http.ResponseWriter, page string, data map[string]any) {
	t, ok := w.pages[page]
	if !ok {
		log.Printf("нет шаблона %q", page)
		http.Error(rw, "Страница не найдена", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, page+".html", data); err != nil {
		log.Printf("шаблон %s: %v", page, err)
		http.Error(rw, "Не удалось показать страницу, попробуйте ещё раз", http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Write(buf.Bytes())
}
