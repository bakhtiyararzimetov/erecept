package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"erecept/internal/db/dbtest"
	"erecept/internal/seed"
)

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	pool := dbtest.New(t)
	if err := seed.Run(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return New(pool).Routes()
}

func login(h http.Handler, user, pass string) *httptest.ResponseRecorder {
	form := url.Values{"login": {user}, "password": {pass}}
	req := httptest.NewRequest("POST", "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func get(h http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestLoginDoctor(t *testing.T) {
	h := newTestServer(t)
	w := login(h, "doctor1", "doctor123")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/doctor" {
		t.Fatalf("код %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	cookies := w.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != "erecept_session" || !cookies[0].HttpOnly {
		t.Fatalf("cookie сессии не выставлена: %v", cookies)
	}

	w = get(h, "/doctor", cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("/doctor с cookie: код %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	if w = get(h, "/doctor", nil); w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/login" {
		t.Errorf("/doctor без cookie: код %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	if w = get(h, "/static/style.css", nil); w.Code != http.StatusOK {
		t.Errorf("style.css: код %d", w.Code)
	}
}

func TestLoginWrong(t *testing.T) {
	h := newTestServer(t)
	w := login(h, "doctor1", "wrong")
	if w.Code != http.StatusOK {
		t.Fatalf("код %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Неверный логин или пароль") {
		t.Error("нет сообщения об ошибке")
	}
	if !strings.Contains(body, `value="doctor1"`) {
		t.Error("логин не сохранился в поле")
	}
}

func TestPharmacyCannotOpenDoctor(t *testing.T) {
	h := newTestServer(t)
	w := login(h, "apteka1", "apteka123")
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/pharmacy" {
		t.Fatalf("вход аптеки: код %d, Location %q", w.Code, w.Header().Get("Location"))
	}
	w = get(h, "/doctor", w.Result().Cookies())
	if w.Code != http.StatusForbidden && !(w.Code == http.StatusSeeOther && w.Header().Get("Location") == "/pharmacy") {
		t.Errorf("аптека открыла /doctor: код %d", w.Code)
	}
}
