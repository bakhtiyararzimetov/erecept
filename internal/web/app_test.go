package web

import (
	"net/http"
	"strings"
	"testing"
)

// Веб-версия приложения пациента: /app — сам файл или index.html (маршруты решает приложение).
func TestPatientWebApp(t *testing.T) {
	h := newTestServer(t)
	for _, p := range []string{"/app", "/app/", "/app/scan?code=abc123", "/app/explain"} {
		w := get(h, p, nil)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "/app/_expo/static/js/") {
			t.Errorf("%s: код %d", p, w.Code)
		}
	}
	w := get(h, "/app/favicon.ico", nil)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "<html") {
		t.Errorf("favicon: код %d", w.Code)
	}
}

// На странице рецепта есть кнопка «открыть в приложении» с кодом рецепта и языком страницы.
func TestPublicPageOpensWebApp(t *testing.T) {
	w := dictateEnv(t, nil)
	h := w.Routes()
	_, code := createRx(t, w.DB, h, doctorCookies(t, h, "doctor1"))
	if body := getLang(h, "/r/"+code+"?lang=en", nil); !strings.Contains(body, `href="/app/scan?code=`+code+`&lang=en"`) || !strings.Contains(body, "Open in the app") {
		t.Error("нет кнопки «Open in the app»")
	}
}

func TestMedicinePackImages(t *testing.T) {
	h := newTestServer(t)
	if w := get(h, "/static/meds/glucophage.png", nil); w.Code != http.StatusOK || w.Header().Get("Content-Type") != "image/png" {
		t.Errorf("картинка упаковки: код %d %q", w.Code, w.Header().Get("Content-Type"))
	}
}
