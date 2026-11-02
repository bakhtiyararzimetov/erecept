package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"erecept/internal/db/dbtest"
	"erecept/internal/seed"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestEnv(t *testing.T) (*pgxpool.Pool, http.Handler) {
	t.Helper()
	pool := dbtest.New(t)
	if err := seed.Run(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return pool, New(pool).Routes()
}

func doctorCookies(t *testing.T, h http.Handler, user string) []*http.Cookie {
	t.Helper()
	w := login(h, user, "doctor123")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("вход %s: код %d", user, w.Code)
	}
	return w.Result().Cookies()
}

func postForm(h http.Handler, path string, form url.Values, cookies []*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func validForm() url.Values {
	return url.Values{
		"patient_name":           {"Иванов Иван Иванович"},
		"patient_birth":          {"12.03.1985"},
		"comment":                {"Больше пить воды"},
		"water_every_hours":      {"2"},
		"water_from":             {"09:00"},
		"water_to":               {"21:00"},
		"items[0].name":          {"Парацетамол"},
		"items[0].dosage":        {"1 таблетка 500 мг"},
		"items[0].times_per_day": {"2"},
		"items[0].days":          {"5"},
		"items[0].meal":          {"after"},
		"items[0].times":         {"08:00, 20:00"},
		"items[1].name":          {"Неизвестное средство"},
		"items[1].dosage":        {"10 капель"},
		"items[1].times_per_day": {"1"},
		"items[1].days":          {"7"},
		"items[1].meal":          {"before"},
		"items[1].times":         {"09:30"},
	}
}

// createRx создаёт рецепт от doctor1 и возвращает id и код.
func createRx(t *testing.T, pool *pgxpool.Pool, h http.Handler, cookies []*http.Cookie) (int64, string) {
	t.Helper()
	w := postForm(h, "/doctor/prescriptions", validForm(), cookies)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("создание: код %d, тело %s", w.Code, w.Body.String())
	}
	var id int64
	var code string
	if err := pool.QueryRow(context.Background(), `SELECT id, code FROM prescriptions ORDER BY id DESC LIMIT 1`).Scan(&id, &code); err != nil {
		t.Fatal(err)
	}
	return id, code
}

func TestCreatePrescription(t *testing.T) {
	pool, h := newTestEnv(t)
	cookies := doctorCookies(t, h, "doctor1")
	w := postForm(h, "/doctor/prescriptions", validForm(), cookies)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("код %d, тело %s", w.Code, w.Body.String())
	}
	var id int64
	var code string
	if err := pool.QueryRow(context.Background(), `SELECT id, code FROM prescriptions`).Scan(&id, &code); err != nil {
		t.Fatal(err)
	}
	if loc := w.Header().Get("Location"); loc != "/doctor/prescriptions/"+itoa(id) {
		t.Errorf("Location %q", loc)
	}
	if len(code) != 10 || code != strings.ToLower(code) || strings.Contains(code, "=") {
		t.Errorf("код рецепта %q", code)
	}
	rows, err := pool.Query(context.Background(), `SELECT medicine_name, medicine_id IS NOT NULL, times, meal FROM prescription_items WHERE prescription_id = $1 ORDER BY position`, id)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type row struct {
		name  string
		hasID bool
		times []string
		meal  string
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.name, &r.hasID, &r.times, &r.meal); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("строк лекарств %d", len(got))
	}
	if got[0].name != "Парацетамол" || !got[0].hasID || strings.Join(got[0].times, ",") != "08:00,20:00" || got[0].meal != "after" {
		t.Errorf("первая строка: %+v", got[0])
	}
	if got[1].hasID || strings.Join(got[1].times, ",") != "09:30" {
		t.Errorf("вторая строка: %+v", got[1])
	}

	// Страница рецепта и список открываются.
	if w := get(h, "/doctor/prescriptions/"+itoa(id), cookies); w.Code != 200 || !strings.Contains(w.Body.String(), "Парацетамол") || !strings.Contains(w.Body.String(), "Соблюдение лечения") {
		t.Errorf("страница рецепта: код %d", w.Code)
	}
	if w := get(h, "/doctor/prescriptions", cookies); w.Code != 200 || !strings.Contains(w.Body.String(), "Ожидает сканирования") {
		t.Errorf("список: код %d", w.Code)
	}
	if w := get(h, "/doctor/prescriptions/"+itoa(id)+"/print", cookies); w.Code != 200 || !strings.Contains(w.Body.String(), "Отсканируйте в приложении еРецепт") || !strings.Contains(w.Body.String(), "2 раза в день, 08:00 и 20:00, после еды") {
		t.Errorf("печать: код %d", w.Code)
	}
	if w := get(h, "/doctor/item-row?i=3", cookies); w.Code != 200 || !strings.Contains(w.Body.String(), `items[3].name`) || strings.Contains(w.Body.String(), "<html") {
		t.Errorf("item-row: код %d", w.Code)
	}
	if w := get(h, "/doctor", cookies); w.Code != 200 || !strings.Contains(w.Body.String(), "QR появится после сохранения") {
		t.Errorf("форма: код %d", w.Code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestCreatePrescriptionInvalid(t *testing.T) {
	pool, h := newTestEnv(t)
	cookies := doctorCookies(t, h, "doctor1")
	cases := []struct {
		name   string
		mutate func(v url.Values)
		want   string
	}{
		{"пустое имя пациента", func(v url.Values) { v.Set("patient_name", " ") }, "Укажите ФИО пациента"},
		{"дата рождения в будущем", func(v url.Values) { v.Set("patient_birth", "2999-01-01") }, "Дата рождения не может быть в будущем"},
		{"дата рождения не в формате", func(v url.Values) { v.Set("patient_birth", "31.02.1985") }, "введите в формате ДД.ММ.ГГГГ"},
		{"раз в день не совпадает с числом времён", func(v url.Values) { v.Set("items[0].times", "08:00") }, "число времён приёма должно совпадать"},
		{"время 25:00", func(v url.Values) { v.Set("items[1].times", "25:00") }, "«25:00» — время нужно в формате ЧЧ:ММ"},
		{"пустое название лекарства", func(v url.Values) { v.Set("items[0].name", "") }, "Укажите название лекарства"},
		{"нет лекарств", func(v url.Values) {
			for k := range v {
				if strings.HasPrefix(k, "items[") {
					v.Del(k)
				}
			}
		}, "Добавьте хотя бы одно лекарство"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := validForm()
			c.mutate(v)
			w := postForm(h, "/doctor/prescriptions", v, cookies)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("код %d", w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, c.want) {
				t.Errorf("нет текста %q", c.want)
			}
			if c.name != "пустое имя пациента" && !strings.Contains(body, "Иванов Иван Иванович") {
				t.Errorf("введённое ФИО не сохранено")
			}
			if c.name == "время 25:00" && !strings.Contains(body, "Неизвестное средство") {
				t.Errorf("поля лекарств не сохранены")
			}
		})
	}
	var n int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM prescriptions`).Scan(&n); err != nil || n != 0 {
		t.Errorf("рецептов в базе %d, ошибка %v", n, err)
	}
}

func TestQRPNG(t *testing.T) {
	pool, h := newTestEnv(t)
	_, code := createRx(t, pool, h, doctorCookies(t, h, "doctor1"))
	w := get(h, "/qr/"+code+".png", nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !strings.HasPrefix(w.Body.String(), "\x89PNG") {
		t.Fatalf("код %d, тип %q", w.Code, w.Header().Get("Content-Type"))
	}
	if w := get(h, "/qr/nonexistent.png", nil); w.Code != 404 {
		t.Errorf("несуществующий код: %d", w.Code)
	}
}

func TestPublicPage(t *testing.T) {
	pool, h := newTestEnv(t)
	_, code := createRx(t, pool, h, doctorCookies(t, h, "doctor1"))
	w := get(h, "/r/"+code, nil)
	body := w.Body.String()
	if w.Code != 200 || !strings.Contains(body, "Парацетамол") || !strings.Contains(body, "после еды") {
		t.Fatalf("код %d", w.Code)
	}
	if w := get(h, "/r/nonexistent", nil); w.Code != 404 {
		t.Errorf("несуществующий код: %d", w.Code)
	}
}

func TestOtherDoctorGets404(t *testing.T) {
	pool, h := newTestEnv(t)
	id, _ := createRx(t, pool, h, doctorCookies(t, h, "doctor1"))
	other := doctorCookies(t, h, "doctor2")
	for _, p := range []string{"", "/print"} {
		if w := get(h, "/doctor/prescriptions/"+itoa(id)+p, other); w.Code != 404 {
			t.Errorf("чужой рецепт %q: код %d", p, w.Code)
		}
	}
}

func TestFormRoundTripFixes(t *testing.T) {
	pool, h := newTestEnv(t)
	cookies := doctorCookies(t, h, "doctor1")

	t.Run("индекс новой строки после удаления", func(t *testing.T) {
		v := validForm()
		for _, k := range []string{"name", "dosage", "times_per_day", "days", "meal", "times"} {
			v.Set("items[2]."+k, v.Get("items[1]."+k))
			v.Del("items[1]." + k)
		}
		v.Set("patient_name", "")
		w := postForm(h, "/doctor/prescriptions", v, cookies)
		if w.Code != http.StatusUnprocessableEntity || !regexp.MustCompile(`window\.rowN = \s*3\s*;`).MatchString(w.Body.String()) {
			t.Fatalf("код %d, rowN не 3", w.Code)
		}
	})

	t.Run("не больше 20 лекарств", func(t *testing.T) {
		v := validForm()
		for i := 2; i < 21; i++ {
			for _, k := range []string{"name", "dosage", "times_per_day", "days", "meal", "times"} {
				v.Set("items["+itoa(int64(i))+"]."+k, v.Get("items[1]."+k))
			}
		}
		w := postForm(h, "/doctor/prescriptions", v, cookies)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "Не больше 20 лекарств в одном рецепте") {
			t.Fatalf("код %d", w.Code)
		}
	})

	t.Run("раз в день вне списка остаётся выбранным", func(t *testing.T) {
		v := validForm()
		v.Set("items[0].times_per_day", "8")
		w := postForm(h, "/doctor/prescriptions", v, cookies)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `<option value="8" selected>8</option>`) {
			t.Fatalf("код %d, значение 8 сброшено", w.Code)
		}
	})

	t.Run("время сортируется", func(t *testing.T) {
		v := validForm()
		v.Set("items[0].times", "20:00, 08:00")
		id, _ := createRxWith(t, pool, h, cookies, v)
		var times []string
		if err := pool.QueryRow(context.Background(), `SELECT times FROM prescription_items WHERE prescription_id=$1 AND position=0`, id).Scan(&times); err != nil || strings.Join(times, ",") != "08:00,20:00" {
			t.Errorf("times %v, ошибка %v", times, err)
		}
	})

	t.Run("заголовки публичных страниц", func(t *testing.T) {
		_, code := createRx(t, pool, h, cookies)
		for _, p := range []string{"/r/" + code, "/qr/" + code + ".png"} {
			w := get(h, p, nil)
			if w.Header().Get("X-Robots-Tag") != "noindex" || w.Header().Get("Cache-Control") != "private, max-age=300" {
				t.Errorf("%s: заголовки %v", p, w.Header())
			}
		}
	})
}

func createRxWith(t *testing.T, pool *pgxpool.Pool, h http.Handler, cookies []*http.Cookie, v url.Values) (int64, string) {
	t.Helper()
	if w := postForm(h, "/doctor/prescriptions", v, cookies); w.Code != http.StatusSeeOther {
		t.Fatalf("создание: код %d", w.Code)
	}
	var id int64
	var code string
	if err := pool.QueryRow(context.Background(), `SELECT id, code FROM prescriptions ORDER BY id DESC LIMIT 1`).Scan(&id, &code); err != nil {
		t.Fatal(err)
	}
	return id, code
}

func TestParseBirthBothFormats(t *testing.T) {
	for _, in := range []string{"12.03.1985", "1985-03-12"} {
		b, ok := parseBirth(in)
		if !ok || b.Format("2006-01-02") != "1985-03-12" {
			t.Fatalf("%q -> %v %v", in, b, ok)
		}
	}
	for _, in := range []string{"", "03/12/1985", "31.02.1985", "abc"} {
		if _, ok := parseBirth(in); ok {
			t.Fatalf("%q не должна приниматься", in)
		}
	}
}

func TestPrescriptionPageShowsExplanation(t *testing.T) {
	pool, h := newTestEnv(t)
	cookies := doctorCookies(t, h, "doctor1")
	id, code := createRx(t, pool, h, cookies)

	// Без памятки блока нет.
	if body := get(h, "/doctor/prescriptions/"+itoa(id), cookies).Body.String(); strings.Contains(body, "Коротко о лекарстве") {
		t.Fatal("блок без памятки не показывается")
	}

	_, err := pool.Exec(context.Background(), `INSERT INTO medicine_explanations (medicine_id, data)
		SELECT id, $1 FROM medicines WHERE name = 'Парацетамол'`, `{
		"purpose": "Снижает температуру и уменьшает боль.",
		"how_to_take": "По 1 таблетке после еды.",
		"do_not_take_if": ["Тяжёлые болезни печени"],
		"missed_dose": "Не принимайте пропущенную дозу самостоятельно — позвоните врачу.",
		"if_stopped_early": "Можно прекратить, когда симптомы прошли.",
		"urgent_signs": ["Сыпь, отёк лица — вызовите 103"],
		"high_risk": true,
		"source": {"title": "Инструкция по медицинскому применению (НЦЭЛС)", "reg_number": "РК-ЛС-5№000042", "url": "https://register.ndda.kz/"},
		"model": "claude-opus-5-5", "generated_at": "2026-11-02T10:00:00Z"}`)
	if err != nil {
		t.Fatal(err)
	}
	w := get(h, "/doctor/prescriptions/"+itoa(id), cookies)
	body := w.Body.String()
	if w.Code != 200 {
		t.Fatalf("код %d", w.Code)
	}
	for _, want := range []string{"Коротко о лекарстве (ИИ)", "Сгенерировано ИИ по инструкции НЦЭЛС, рег. № РК-ЛС-5№000042",
		"Не заменяет совет врача", "Снижает температуру и уменьшает боль.", "Тяжёлые болезни печени",
		"позвоните врачу", "Сыпь, отёк лица", `class="explain-note"`} {
		if !strings.Contains(body, want) {
			t.Errorf("на странице нет %q", want)
		}
	}
	// Памятка только у Парацетамола, у «Неизвестного средства» её нет.
	if n := strings.Count(body, "Коротко о лекарстве (ИИ)"); n != 1 {
		t.Errorf("блоков памятки %d, ожидался 1", n)
	}
	// Публичная страница и печать памятку не показывают.
	if strings.Contains(get(h, "/r/"+code, nil).Body.String(), "Коротко о лекарстве") {
		t.Error("на публичной странице памятки быть не должно")
	}
}
