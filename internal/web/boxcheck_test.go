package web

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erecept/internal/boxcheck"
)

type fakeBoxChecker struct {
	lang  boxcheck.Lang
	res   boxcheck.Result
	err   error
	items []boxcheck.Item
	img   []byte
}

func (f *fakeBoxChecker) Check(_ context.Context, img []byte, items []boxcheck.Item, lang boxcheck.Lang) (boxcheck.Result, error) {
	f.img, f.items, f.lang = img, items, lang
	return f.res, f.err
}

// boxEnv — рецепт от doctor1 (Парацетамол из справочника + средство вне справочника) и проверка упаковки.
func boxEnv(t *testing.T, c BoxChecker) (*Web, http.Handler, string) {
	t.Helper()
	w := dictateEnv(t, nil)
	if c != nil {
		w.BoxCheck = c
	}
	h := w.Routes()
	_, code := createRx(t, w.DB, h, doctorCookies(t, h, "doctor1"))
	return w, h, code
}

func postPhoto(h http.Handler, path string, photo []byte, ip string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if photo != nil {
		fw, _ := mw.CreateFormFile("photo", "box.jpg")
		fw.Write(photo)
	}
	mw.Close()
	req := httptest.NewRequest("POST", path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.RemoteAddr = ip + ":5555"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestBoxCheckShowsVerdict(t *testing.T) {
	fake := &fakeBoxChecker{res: boxcheck.Result{Verdict: "analog", Item: "Парацетамол", Message: "✅ Аналог: другое название, то же вещество"}}
	_, h, code := boxEnv(t, fake)
	w := postPhoto(h, "/r/"+code+"/check", []byte("photo-bytes"), "10.0.0.1")
	if w.Code != http.StatusOK {
		t.Fatalf("код %d, тело %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "✅ Аналог") || !strings.Contains(body, boxcheck.Footer) || !strings.Contains(body, "box-result ok") {
		t.Errorf("тело: %s", body)
	}
	if string(fake.img) != "photo-bytes" {
		t.Errorf("фото не дошло: %q", fake.img)
	}
	if len(fake.items) != 2 || fake.items[0].Name != "Парацетамол" || fake.items[0].Substance != "парацетамол" ||
		fake.items[0].Dosage != "1 таблетка 500 мг" || fake.items[1].Substance != "" {
		t.Errorf("строки рецепта: %+v", fake.items)
	}
}

func TestBoxCheckOffAndNotFound(t *testing.T) {
	_, h, code := boxEnv(t, nil)
	if w := postPhoto(h, "/r/"+code+"/check", []byte("x"), "10.0.0.2"); w.Code != http.StatusNotFound {
		t.Errorf("без проверки: код %d", w.Code)
	}
	if strings.Contains(get(h, "/r/"+code, nil).Body.String(), "Проверить упаковку") {
		t.Error("кнопка видна, хотя проверка выключена")
	}

	_, h, code = boxEnv(t, &fakeBoxChecker{})
	if w := postPhoto(h, "/r/nosuchcode/check", []byte("x"), "10.0.0.2"); w.Code != http.StatusNotFound {
		t.Errorf("чужой код: %d", w.Code)
	}
	if !strings.Contains(get(h, "/r/"+code, nil).Body.String(), "Проверить упаковку") {
		t.Error("нет кнопки на публичной странице")
	}
}

func TestBoxCheckBadUpload(t *testing.T) {
	_, h, code := boxEnv(t, &fakeBoxChecker{err: boxcheck.ErrImage})
	for name, photo := range map[string][]byte{"без фото": nil, "не картинка": []byte("x")} {
		w := postPhoto(h, "/r/"+code+"/check", photo, "10.0.0.3")
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Сфотографируйте") {
			t.Errorf("%s: код %d, тело %s", name, w.Code, w.Body.String())
		}
	}
}

func TestBoxCheckRateLimit(t *testing.T) {
	_, h, code := boxEnv(t, &fakeBoxChecker{res: boxcheck.Result{Verdict: "match", Message: "ok"}})
	for i := 0; i < 10; i++ {
		if w := postPhoto(h, "/r/"+code+"/check", []byte("x"), "10.0.0.4"); w.Code != http.StatusOK {
			t.Fatalf("проверка %d: код %d", i+1, w.Code)
		}
	}
	w := postPhoto(h, "/r/"+code+"/check", []byte("x"), "10.0.0.4")
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "Слишком много проверок") {
		t.Errorf("11-я: код %d, тело %s", w.Code, w.Body.String())
	}
	if w := postPhoto(h, "/r/"+code+"/check", []byte("x"), "10.0.0.5"); w.Code != http.StatusOK {
		t.Errorf("другой телефон: код %d", w.Code)
	}
}

func TestBoxCheckAIError(t *testing.T) {
	_, h, code := boxEnv(t, &fakeBoxChecker{err: errors.New("Claude API: 500")})
	w := postPhoto(h, "/r/"+code+"/check", []byte("x"), "10.0.0.6")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "ИИ сейчас не ответил") {
		t.Errorf("код %d, тело %s", w.Code, w.Body.String())
	}
}

func TestRateLimitGlobal(t *testing.T) {
	l := newRateLimit(10, time.Hour, 3, 24*time.Hour)
	now := time.Now()
	for i, ip := range []string{"a", "b", "c"} {
		if !l.allow(ip, now) {
			t.Fatalf("запрос %d отклонён", i+1)
		}
	}
	if l.allow("d", now) {
		t.Error("общий лимит не сработал")
	}
	if !l.allow("d", now.Add(25*time.Hour)) {
		t.Error("лимит не сбросился через сутки")
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{"без прокси", nil, "10.1.1.1:5555", "10.1.1.1"},
		{"Cloudflare", map[string]string{"CF-Connecting-IP": "5.5.5.5", "X-Forwarded-For": "6.6.6.6, 7.7.7.7"}, "10.0.0.1:1", "5.5.5.5"},
		{"True-Client-IP", map[string]string{"True-Client-IP": "8.8.8.8"}, "10.0.0.1:1", "8.8.8.8"},
		{"XFF — последний адрес", map[string]string{"X-Forwarded-For": "1.2.3.4, 9.9.9.9"}, "10.0.0.1:1", "9.9.9.9"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/", nil)
		r.RemoteAddr = c.remote
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := clientIP(r); got != c.want {
			t.Errorf("%s: %q, нужно %q", c.name, got, c.want)
		}
	}
}

// Выключенная проверка отвечает тем же оформленным куском, а не голым «404 page not found».
func TestBoxCheckOffReturnsFragment(t *testing.T) {
	_, h, code := boxEnv(t, nil)
	w := postPhoto(h, "/r/"+code+"/check", []byte("x"), "10.0.0.7")
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "box-result") {
		t.Errorf("код %d, тело %s", w.Code, w.Body.String())
	}
}

func TestBoxCheckEnglish(t *testing.T) {
	fake := &fakeBoxChecker{res: boxcheck.Result{Verdict: "match", Item: "Парацетамол", Message: "✅ Matches your prescription: Paracetamol."}}
	_, h, code := boxEnv(t, fake)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("photo", "box.jpg")
	fw.Write([]byte("x"))
	mw.Close()
	req := httptest.NewRequest("POST", "/r/"+code+"/check", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "lang", Value: "en"})
	req.RemoteAddr = "10.0.0.9:1"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), boxcheck.FooterEN) {
		t.Errorf("нет английской подписи: %s", w.Body.String())
	}
	if fake.lang != boxcheck.EN || fake.items[0].NameEN != "Paracetamol" || fake.items[0].SubstanceEN != "paracetamol" {
		t.Errorf("язык %q, строки %+v", fake.lang, fake.items)
	}
}
