package web

import (
	"io/fs"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"erecept/internal/i18n"
)

// Каждая строка {{t "…"}} / {{tf "…"}} в шаблонах переведена на английский.
func TestTemplatesTranslated(t *testing.T) {
	re := regexp.MustCompile(`\{\{tf? "([^"]+)"`)
	files, _ := fs.Glob(assets, "templates/*.html")
	n := 0
	for _, f := range files {
		b, _ := fs.ReadFile(assets, f)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			n++
			if !i18n.Has(m[1]) {
				t.Errorf("%s: нет перевода для %q", f, m[1])
			}
		}
	}
	if n < 50 {
		t.Errorf("найдено всего %d строк — регулярка сломалась?", n)
	}
}

func getLang(h http.Handler, path string, cookies []*http.Cookie) string {
	return get(h, path, cookies).Body.String()
}

func TestEnglishPublicPage(t *testing.T) {
	w := dictateEnv(t, nil)
	w.BoxCheck = &fakeBoxChecker{}
	h := w.Routes()
	_, code := createRx(t, w.DB, h, doctorCookies(t, h, "doctor1"))
	body := getLang(h, "/r/"+code+"?lang=en", nil)
	for _, want := range []string{"<h1>Prescription</h1>", "Doctor:", "Check the box", "2 times a day, 08:00 and 20:00, after meals", "Course: 5 days", `lang="en"`} {
		if !strings.Contains(body, want) {
			t.Errorf("нет %q", want)
		}
	}
	if strings.Contains(body, "<h1>Рецепт</h1>") || strings.Contains(body, "Проверить упаковку") {
		t.Error("на английской странице остался русский интерфейс")
	}
	if ru := getLang(h, "/r/"+code, nil); !strings.Contains(ru, "<h1>Рецепт</h1>") {
		t.Error("без lang страница должна быть русской")
	}
}

func TestEnglishDoctorForm(t *testing.T) {
	w := dictateEnv(t, &fakeDictator{})
	h := w.Routes()
	body := getLang(h, "/doctor/?lang=en", doctorCookies(t, h, "doctor1"))
	for _, want := range []string{"New prescription", "Dictate the prescription (AI)", "Fill in with AI", "Save prescription", `"en-US"`} {
		if !strings.Contains(body, want) {
			t.Errorf("нет %q", want)
		}
	}
}
