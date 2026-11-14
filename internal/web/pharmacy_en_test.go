package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestPharmacyStats(t *testing.T) {
	pool, h := newTestEnv(t)
	c := pharmacyCookies(t, h, "apteka1")
	var total, in, out int
	err := pool.QueryRow(context.Background(), `SELECT count(*), count(*) FILTER (WHERE i.in_stock), count(*) FILTER (WHERE NOT i.in_stock)
		FROM pharmacy_items i JOIN pharmacies p ON p.id = i.pharmacy_id WHERE p.login = 'apteka1'`).Scan(&total, &in, &out)
	if err != nil {
		t.Fatal(err)
	}
	body := get(h, "/pharmacy", c).Body.String()
	for _, want := range []string{
		`id="stat-total">` + itoa(int64(total)) + `<`,
		`id="stat-in">` + itoa(int64(in)) + `<`,
		`id="stat-out">` + itoa(int64(out)) + `<`,
		"Всего позиций", "В наличии", "Нет в наличии",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("нет %q", want)
		}
	}
	// Счётчики — по всему прайсу, даже когда в поиске одна строка.
	if body := get(h, "/pharmacy?q=парацет", c).Body.String(); !strings.Contains(body, `id="stat-total">`+itoa(int64(total))+`<`) {
		t.Error("с поиском счётчик «Всего позиций» изменился")
	}
}

func TestPharmacyEnglish(t *testing.T) {
	_, h := newTestEnv(t)
	c := pharmacyCookies(t, h, "apteka1")
	body := get(h, "/pharmacy?lang=en", c).Body.String()
	for _, want := range []string{"<h1>My medicines</h1>", "Glucophage", "tablets 850 mg, 60 pcs", "metformin", "Add medicine", "Upload CSV", "In stock", "Out of stock", `<option value="Glucophage">`, "Pharmacy «Здоровье плюс»", "Shymkent"} {
		if !strings.Contains(body, want) {
			t.Errorf("нет %q", want)
		}
	}
	// Интерфейс и справочник по-английски: кириллица только в имени собственном аптеки.
	text := regexp.MustCompile(`(?s)<style.*?</style>|<script.*?</script>|<[^>]+>`).ReplaceAllString(body, " ")
	text = strings.NewReplacer("Здоровье плюс", "", "Тауке хана", "").Replace(text)
	if m := regexp.MustCompile(`[А-Яа-яЁё]+`).FindAllString(text, 10); len(m) > 0 {
		t.Errorf("на английской странице русский текст: %v", m)
	}

	ru := get(h, "/pharmacy?lang=ru", c).Body.String()
	for _, want := range []string{"Глюкофаж", "таблетки 850 мг №60", "Добавить лекарство"} {
		if !strings.Contains(ru, want) {
			t.Errorf("RU: нет %q", want)
		}
	}
	if strings.Contains(ru, "Glucophage") {
		t.Error("RU: английское название на русской странице")
	}

	// Поиск понимает английское название.
	tbody := func(path string) string {
		body := get(h, path, c).Body.String()
		_, after, _ := strings.Cut(body, `id="items-body"`)
		list, _, _ := strings.Cut(after, "</tbody>")
		return list
	}
	if list := tbody("/pharmacy?lang=en&q=gluco"); !strings.Contains(list, "Glucophage") || strings.Contains(list, "Amlodipine") {
		t.Error("EN поиск «gluco» не нашёл Glucophage")
	}
	if list := tbody("/pharmacy?lang=ru&q=gluco"); !strings.Contains(list, "Глюкофаж") {
		t.Error("RU поиск по английскому названию не нашёл Глюкофаж")
	}
}

// Английское название в форме и в CSV — то же лекарство справочника, без дублей.
func TestPharmacyEnglishNameLinksCatalog(t *testing.T) {
	pool, h := newTestEnv(t)
	ctx := context.Background()
	c := pharmacyCookies(t, h, "apteka2")
	count := func(q string, args ...any) int {
		var n int
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	meds := count(`SELECT count(*) FROM medicines`)

	w := postForm(h, "/pharmacy/items?lang=en", url.Values{"name": {"glucophage"}, "price": {"3333"}, "in_stock": {"1"}}, c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Saved") {
		t.Fatalf("добавление: код %d", w.Code)
	}
	if n := count(`SELECT count(*) FROM pharmacy_items i JOIN medicines m ON m.id = i.medicine_id JOIN pharmacies p ON p.id = i.pharmacy_id
		WHERE p.login = 'apteka2' AND m.name = 'Глюкофаж' AND i.price = 3333`); n != 1 {
		t.Error("цена не попала на Глюкофаж")
	}

	w = postCSV(h, "name;price;in stock\nAmlodipine;650;yes\nOmez;2400;no\n", c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Загружено 2, ошибок 0") {
		t.Fatalf("CSV: код %d %s", w.Code, w.Body.String())
	}
	if n := count(`SELECT count(*) FROM pharmacy_items i JOIN medicines m ON m.id = i.medicine_id JOIN pharmacies p ON p.id = i.pharmacy_id
		WHERE p.login = 'apteka2' AND ((m.name = 'Амлодипин' AND i.price = 650 AND i.in_stock) OR (m.name = 'Омез' AND i.price = 2400 AND NOT i.in_stock))`); n != 2 {
		t.Errorf("CSV с английскими названиями: обновлено %d из 2", n)
	}
	// В английском кабинете ошибки CSV — по-английски.
	w = postCSV(h, "name;price\nAmlodipine;abc\n", append(c, &http.Cookie{Name: "lang", Value: "en"}))
	if body := w.Body.String(); !strings.Contains(body, "Uploaded 0, errors 1") || !strings.Contains(body, "line 2: price «abc» is not a number") {
		t.Errorf("английские ошибки CSV: %d", w.Code)
	}
	if n := count(`SELECT count(*) FROM medicines`); n != meds {
		t.Errorf("в справочнике появились дубли: %d -> %d", meds, n)
	}
}

// Строка после сохранения (htmx) — на языке страницы, наличие сохраняется переключателем.
func TestPharmacyRowEnglish(t *testing.T) {
	pool, h := newTestEnv(t)
	c := pharmacyCookies(t, h, "apteka1")
	var id int64
	err := pool.QueryRow(context.Background(), `SELECT i.id FROM pharmacy_items i JOIN medicines m ON m.id = i.medicine_id
		JOIN pharmacies p ON p.id = i.pharmacy_id WHERE p.login = 'apteka1' AND m.name = 'Глюкофаж'`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "/pharmacy/items/"+itoa(id)+"?lang=en", strings.NewReader("price=2900"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	for _, ck := range c {
		req.AddCookie(ck)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "Glucophage") || !strings.Contains(body, "tablets 850 mg, 60 pcs") || !strings.Contains(body, "Out of stock") {
		t.Fatalf("строка: код %d %s", rec.Code, body)
	}
	if !strings.Contains(body, `hx-trigger="change"`) {
		t.Error("переключатель наличия не сохраняется сам")
	}
}
