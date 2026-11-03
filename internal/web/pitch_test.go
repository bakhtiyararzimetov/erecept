package web

import (
	"strings"
	"testing"
)

// Презентация для жюри открывается по короткому адресу без входа.
func TestPitchPage(t *testing.T) {
	h := newTestServer(t)

	w := get(h, "/pitch", nil)
	if w.Code != 302 || w.Header().Get("Location") != "/static/pitch/" {
		t.Fatalf("/pitch: код %d, Location %q", w.Code, w.Header().Get("Location"))
	}

	w = get(h, "/static/pitch/", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "eRecept") {
		t.Fatalf("/static/pitch/: код %d", w.Code)
	}

	w = get(h, "/static/pitch/img/04-today.png", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("картинка: код %d, тип %q", w.Code, w.Header().Get("Content-Type"))
	}
}
