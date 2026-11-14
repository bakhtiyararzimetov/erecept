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

// Имена в английском режиме — латиницей, клиника — по-английски; на русском — как есть.
func TestPersonAndClinicEN(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Ахметова Динара Сериковна", "Akhmetova Dinara Serikovna"},
		{"Ким Александр Петрович", "Kim Aleksandr Petrovich"},
		{"Иванова Галина Петровна", "Ivanova Galina Petrovna"},
		{"Жанна Щукина", "Zhanna Shchukina"},
		{"Smith John", "Smith John"},
	}
	for _, c := range cases {
		if got := Person(EN, c.in); got != c.want {
			t.Errorf("Person(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := Person(RU, c.in); got != c.in {
			t.Errorf("RU Person(%q) = %q", c.in, got)
		}
	}
	if got := Clinic(EN, "Поликлиника №5"); got != "Polyclinic No. 5" {
		t.Errorf("clinic: %q", got)
	}
	if got := Clinic(EN, "Городская поликлиника №12"); got != "City Polyclinic No. 12" {
		t.Errorf("clinic: %q", got)
	}
	if got := Clinic(RU, "Поликлиника №5"); got != "Поликлиника №5" {
		t.Errorf("clinic RU: %q", got)
	}
}

func TestDosageEN(t *testing.T) {
	cases := map[string]string{
		"1 таблетка 500 мг": "1 tablet 500 mg",
		"2 таблетки":        "2 tablets",
		"1 капсула 20 мг":   "1 capsule 20 mg",
		"10 капель":         "10 drops",
		"5 мл сиропа":       "5 ml of syrup",
		"½ таблетки":        "½ tablet",
	}
	for in, want := range cases {
		if got := Dosage(EN, in); got != want {
			t.Errorf("Dosage(%q) = %q, want %q", in, got, want)
		}
	}
	if Dosage(RU, "1 таблетка") != "1 таблетка" {
		t.Error("RU")
	}
}
