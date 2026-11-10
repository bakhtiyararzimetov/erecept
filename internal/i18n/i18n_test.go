package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestT(t *testing.T) {
	if got := T(EN, "Рецепт"); got != "Prescription" {
		t.Errorf("EN: %q", got)
	}
	if got := T(RU, "Рецепт"); got != "Рецепт" {
		t.Errorf("RU: %q", got)
	}
	if got := T(EN, "нет такой строки"); got != "нет такой строки" {
		t.Errorf("нет перевода — русский: %q", got)
	}
	if got := Tf(EN, "Курс: %d дн.", 7); got != "Course: 7 days" {
		t.Errorf("Tf: %q", got)
	}
}

func run(t *testing.T, target string, cookie string) (Lang, *httptest.ResponseRecorder) {
	t.Helper()
	var got Lang
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = FromContext(r.Context()) }))
	req := httptest.NewRequest("GET", target, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return got, w
}

func TestMiddleware(t *testing.T) {
	if l, w := run(t, "/r/x?lang=en", ""); l != EN || w.Header().Get("Set-Cookie") == "" {
		t.Errorf("?lang=en: %s, cookie %q", l, w.Header().Get("Set-Cookie"))
	}
	if l, w := run(t, "/r/x?lang=xx", ""); l != RU || w.Header().Get("Set-Cookie") != "" {
		t.Errorf("?lang=xx: %s, cookie %q", l, w.Header().Get("Set-Cookie"))
	}
	if l, _ := run(t, "/r/x", "en"); l != EN {
		t.Errorf("cookie en: %s", l)
	}
	if l, _ := run(t, "/r/x?lang=ru", "en"); l != RU {
		t.Errorf("?lang=ru сильнее cookie: %s", l)
	}
	if l, _ := run(t, "/r/x", ""); l != RU {
		t.Errorf("по умолчанию: %s", l)
	}
}
