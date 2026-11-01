package web

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func pharmacyCookies(t *testing.T, h http.Handler, user string) []*http.Cookie {
	t.Helper()
	w := login(h, user, "apteka123")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("вход %s: код %d", user, w.Code)
	}
	return w.Result().Cookies()
}

func postCSV(h http.Handler, content string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "price.csv")
	fw.Write([]byte(content))
	mw.Close()
	req := httptest.NewRequest("POST", "/pharmacy/csv", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestPharmacyCabinet(t *testing.T) {
	pool, h := newTestEnv(t)
	ctx := context.Background()
	c1 := pharmacyCookies(t, h, "apteka1")
	c2 := pharmacyCookies(t, h, "apteka2")

	count := func(q string, args ...any) int {
		var n int
		if err := pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("страница и образец", func(t *testing.T) {
		w := get(h, "/pharmacy", c1)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Добавить лекарство") || !strings.Contains(w.Body.String(), "/pharmacy/sample.csv") {
			t.Fatalf("код %d", w.Code)
		}
		w = get(h, "/pharmacy?q=парацет", c1)
		if w.Code != 200 || !strings.Contains(w.Body.String(), "Парацетамол") {
			t.Fatalf("поиск: код %d", w.Code)
		}
		w = get(h, "/pharmacy/sample.csv", c1)
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") || !strings.HasPrefix(w.Body.String(), "\ufeffназвание;") {
			t.Fatalf("образец: %d %q", w.Code, w.Header().Get("Content-Type"))
		}
	})

	t.Run("добавить и обновить", func(t *testing.T) {
		add := func(price string) *httptest.ResponseRecorder {
			return postForm(h, "/pharmacy/items", url.Values{"name": {"  Тестин "}, "substance": {"тестовое"}, "form": {"таблетки"}, "price": {price}, "in_stock": {"1"}}, c1)
		}
		if w := add("1 250 ₸"); w.Code != 200 {
			t.Fatalf("добавление: код %d %s", w.Code, w.Body.String())
		}
		if count(`SELECT count(*) FROM medicines WHERE name='Тестин'`) != 1 {
			t.Fatal("лекарство не создано в справочнике")
		}
		if w := add("900"); w.Code != 200 {
			t.Fatalf("повтор: код %d", w.Code)
		}
		var id int64
		var price int
		if err := pool.QueryRow(ctx, `SELECT pi.id, pi.price FROM pharmacy_items pi JOIN medicines m ON m.id=pi.medicine_id JOIN pharmacies p ON p.id=pi.pharmacy_id WHERE m.name='Тестин' AND p.login='apteka1'`).Scan(&id, &price); err != nil || price != 900 {
			t.Fatalf("цена %d, ошибка %v", price, err)
		}
		if count(`SELECT count(*) FROM pharmacy_items pi JOIN medicines m ON m.id=pi.medicine_id WHERE m.name='Тестин'`) != 1 {
			t.Fatal("дубль позиции")
		}
		// плохая цена: 422, ничего не сломано
		if w := add("abc"); w.Code != http.StatusUnprocessableEntity {
			t.Fatalf("плохая цена: код %d", w.Code)
		}

		path := "/pharmacy/items/" + itoa(id)
		req := httptest.NewRequest("POST", path, strings.NewReader(url.Values{"price": {"1 500"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("HX-Request", "true")
		for _, c := range c1 {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<tr") || !strings.Contains(rec.Body.String(), `value="1500"`) {
			t.Fatalf("обновление: код %d %s", rec.Code, rec.Body.String())
		}
		var stock bool
		pool.QueryRow(ctx, `SELECT price, in_stock FROM pharmacy_items WHERE id=$1`, id).Scan(&price, &stock)
		if price != 1500 || stock {
			t.Fatalf("после обновления price=%d in_stock=%v", price, stock)
		}

		// чужая позиция
		if w := postForm(h, path, url.Values{"price": {"1"}}, c2); w.Code != 404 {
			t.Fatalf("чужая позиция: код %d", w.Code)
		}
		if w := postForm(h, path+"/delete", nil, c2); w.Code != 404 {
			t.Fatalf("чужое удаление: код %d", w.Code)
		}
		if count(`SELECT count(*) FROM pharmacy_items WHERE id=$1`, id) != 1 {
			t.Fatal("чужая аптека удалила позицию")
		}

		if w := postForm(h, path+"/delete", nil, c1); w.Code != http.StatusSeeOther && w.Code != 200 {
			t.Fatalf("удаление: код %d", w.Code)
		}
		if count(`SELECT count(*) FROM pharmacy_items WHERE id=$1`, id) != 0 {
			t.Fatal("позиция не удалена")
		}
	})

	t.Run("загрузка CSV", func(t *testing.T) {
		w := postCSV(h, "\ufeffназвание;цена;наличие\nCSV-один;1 000 ₸;да\nCSV-два;abc;да\nCSV-три;50;нет\n", c1)
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, "Загружено 2, ошибок 1") || !strings.Contains(body, "строка 3") {
			t.Fatalf("код %d, тело: %s", w.Code, body)
		}
		if count(`SELECT count(*) FROM pharmacy_items pi JOIN medicines m ON m.id=pi.medicine_id WHERE m.name LIKE 'CSV-%'`) != 2 {
			t.Fatal("ожидали 2 позиции")
		}
		w = postCSV(h, "название;форма\nА;б\n", c1)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "цена") {
			t.Fatalf("без цены: код %d", w.Code)
		}
	})

	t.Run("справочник не перезаписывается", func(t *testing.T) {
		postForm(h, "/pharmacy/items", url.Values{"name": {"ПАРАЦЕТАМОЛ"}, "substance": {"чужое"}, "form": {"чужая"}, "price": {"10"}}, c1)
		var n int
		pool.QueryRow(ctx, `SELECT count(*) FROM medicines WHERE lower(name)='парацетамол'`).Scan(&n)
		if n != 1 {
			t.Fatalf("дубль лекарства: %d", n)
		}
		if count(`SELECT count(*) FROM medicines WHERE active_substance='чужое' OR form='чужая'`) != 0 {
			t.Fatal("перезаписано непустое значение")
		}
	})
}
