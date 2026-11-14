package web

import (
	"context"
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
	for _, want := range []string{"<h1>Prescription</h1>", "<dt>Doctor</dt>", "Akhmetova Dinara", "Polyclinic No. 5", "Check the box", "2 times a day, 08:00 and 20:00, after meals", "Course: 5 days", `lang="en"`} {
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

// На английской странице — английские названия из справочника; русская страница как была.
func TestEnglishMedicineNames(t *testing.T) {
	w := dictateEnv(t, nil)
	h := w.Routes()
	_, code := createRx(t, w.DB, h, doctorCookies(t, h, "doctor1"))
	if body := getLang(h, "/r/"+code+"?lang=en", nil); !strings.Contains(body, "<h2>Paracetamol</h2>") {
		t.Error("нет английского названия Paracetamol")
	}
	if body := getLang(h, "/r/"+code+"?lang=ru", nil); !strings.Contains(body, "<h2>Парацетамол</h2>") {
		t.Error("на русской странице должно остаться «Парацетамол»")
	}
	if body := getLang(h, "/doctor/?lang=en", doctorCookies(t, h, "doctor1")); !strings.Contains(body, `<option value="Glucophage">`) {
		t.Error("в подсказках формы врача нет английских названий")
	}
}

// Врач вписал английское название — рецепт привязан к тому же лекарству справочника.
func TestDoctorEnglishNameLinksCatalog(t *testing.T) {
	w := dictateEnv(t, nil)
	h := w.Routes()
	f := validForm()
	f.Set("items[0].name", "glucophage")
	if r := postForm(h, "/doctor/prescriptions", f, doctorCookies(t, h, "doctor1")); r.Code != http.StatusSeeOther {
		t.Fatalf("код %d", r.Code)
	}
	var got, want int64
	w.DB.QueryRow(context.Background(), `SELECT medicine_id FROM prescription_items WHERE medicine_name = 'glucophage'`).Scan(&got)
	w.DB.QueryRow(context.Background(), `SELECT id FROM medicines WHERE name = 'Глюкофаж'`).Scan(&want)
	if got == 0 || got != want {
		t.Errorf("medicine_id %d, нужно %d", got, want)
	}
}

func TestEnglishExplanationShown(t *testing.T) {
	w := dictateEnv(t, nil)
	h := w.Routes()
	_, code := createRx(t, w.DB, h, doctorCookies(t, h, "doctor1"))
	_, err := w.DB.Exec(context.Background(), `INSERT INTO medicine_explanations (medicine_id, data, data_en)
		SELECT id, '{"purpose":"Снижает температуру.","how_to_take":"После еды.","missed_dose":"Выпейте.","if_stopped_early":"Можно."}',
		           '{"purpose":"Lowers fever.","how_to_take":"After meals.","missed_dose":"Take it.","if_stopped_early":"You can stop."}'
		FROM medicines WHERE name = 'Парацетамол'`)
	if err != nil {
		t.Fatal(err)
	}
	if body := getLang(h, "/r/"+code+"?lang=en", nil); !strings.Contains(body, "Lowers fever.") || strings.Contains(body, "Снижает температуру.") {
		t.Error("на английской странице нет английской памятки")
	}
	if body := getLang(h, "/r/"+code+"?lang=ru", nil); !strings.Contains(body, "Снижает температуру.") {
		t.Error("на русской странице должна быть русская памятка")
	}
}

// Предупреждение о 20 лекарствах — через data-атрибут: в hx-on шаблон экранирует текст как HTML, а не JS.
func TestMaxItemsAlertIntact(t *testing.T) {
	w := dictateEnv(t, nil)
	h := w.Routes()
	for _, l := range []string{"ru", "en"} {
		body := getLang(h, "/doctor/?lang="+l, doctorCookies(t, h, "doctor1"))
		if !strings.Contains(body, "alert(this.dataset.maxMsg)") || !strings.Contains(body, "data-max-msg=") {
			t.Errorf("%s: предупреждение о 20 лекарствах сломано", l)
		}
	}
	if body := getLang(h, "/doctor/?lang=en", doctorCookies(t, h, "doctor1")); !strings.Contains(body, "Add medicine") {
		t.Error("кнопка «+ Добавить лекарство» не переведена")
	}
}

func TestEnglishPrintPage(t *testing.T) {
	w := dictateEnv(t, nil)
	h := w.Routes()
	cookies := doctorCookies(t, h, "doctor1")
	id, _ := createRx(t, w.DB, h, cookies)
	body := getLang(h, "/doctor/prescriptions/"+itoa(id)+"/print?lang=en", cookies)
	if strings.ContainsAny(stripData(body), "абвгдежзийклмнопрстуфхцчшщыэюя") {
		t.Errorf("на английской печати остался русский текст: %s", stripData(body))
	}
}

// stripData убирает данные рецепта (ФИО, клиника, комментарий), оставляя текст интерфейса.
func stripData(body string) string {
	for _, d := range []string{"Иванов Иван Иванович", "Больше пить воды", "Неизвестное средство", "10 капель", "1 таблетка 500 мг"} {
		body = strings.ReplaceAll(body, d, "")
	}
	re := regexp.MustCompile(`(?s)<style.*?</style>|<[^>]+>`)
	body = re.ReplaceAllString(body, " ")
	for _, n := range []string{"Ахметова Динара Сериковна", "Поликлиника №5", "Сейтказиев", "Ержан"} {
		body = strings.ReplaceAll(body, n, "")
	}
	return body
}
