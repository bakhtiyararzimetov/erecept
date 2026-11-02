package leaflet

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(h http.HandlerFunc) (*Client, func()) {
	srv := httptest.NewServer(h)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client()}, srv.Close
}

func TestFindExactMatch(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/list" {
			t.Errorf("неожиданный запрос: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"id":1,"drugTradeName":"Нурофен® Форте для детей","regNumber":"A","mnn":"ибупрофен"},
			{"id":2,"drugTradeName":" Нурофен® ","regNumber":"РК-ЛС-5№1","mnn":"ибупрофен"}]`))
	})
	defer stop()
	e, err := c.Find(context.Background(), "нурофен")
	if err != nil {
		t.Fatal(err)
	}
	if e.RegisterID != 2 || e.RegNumber != "РК-ЛС-5№1" || e.MNN != "ибупрофен" {
		t.Fatalf("выбрана не та запись: %+v", e)
	}
}

func TestFindFirstWhenNoExact(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":7,"drugTradeName":"Нурофен® Экспресс"},{"id":8,"drugTradeName":"Нурофен® Плюс"}]`))
	})
	defer stop()
	e, err := c.Find(context.Background(), "Нурофен")
	if err != nil || e.RegisterID != 7 {
		t.Fatalf("ожидалась первая запись: %+v, %v", e, err)
	}
}

func TestFindNotFound(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})
	defer stop()
	if _, err := c.Find(context.Background(), "Нет такого"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидался ErrNotFound, получено %v", err)
	}
}

func makeDocx(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="x"><w:body>` + body + `</w:body></w:document>`))
	zw.Close()
	return buf.Bytes()
}

func TestInstructionDocx(t *testing.T) {
	docx := makeDocx(t, `<w:p><w:r><w:t>Показания</w:t></w:r></w:p><w:p></w:p>`+
		`<w:p><w:r><w:t>Способ </w:t></w:r><w:r><w:t>применения &amp; дозы</w:t></w:r></w:p>`)
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getRegisterInstructionFile" || r.URL.Query().Get("registerId") != "42" || r.URL.Query().Get("lang") != "ru" {
			t.Errorf("неожиданный запрос: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		w.Write(docx)
	})
	defer stop()
	text, err := c.Instruction(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Способ применения & дозы") {
		t.Fatalf("нет текста: %q", text)
	}
	if text != "Показания\nСпособ применения & дозы" {
		t.Fatalf("неверное разбиение на абзацы: %q", text)
	}
}

func TestInstructionNotDocx(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>Ошибка</html>`))
	})
	defer stop()
	if _, err := c.Instruction(context.Background(), 1); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

func TestInstructionEmpty(t *testing.T) {
	docx := makeDocx(t, `<w:p></w:p>`)
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) { w.Write(docx) })
	defer stop()
	if _, err := c.Instruction(context.Background(), 1); err == nil {
		t.Fatal("ожидалась ошибка для пустого текста")
	}
}

func TestSourceURL(t *testing.T) {
	if got := SourceURL(Entry{RegisterID: 5}); got != "https://register.ndda.kz/" {
		t.Fatal(got)
	}
}
