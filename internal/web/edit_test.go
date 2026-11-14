package web

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// До скана пациентом рецепт можно исправить: код тот же, лекарства заменены.
func TestEditBeforeScan(t *testing.T) {
	pool, h := newTestEnv(t)
	c := doctorCookies(t, h, "doctor1")
	id, code := createRx(t, pool, h, c)
	w := get(h, "/doctor/prescriptions/"+itoa(id)+"/edit", c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `value="Иванов Иван Иванович"`) || !strings.Contains(w.Body.String(), "Неизвестное средство") {
		t.Fatalf("форма правки: %d", w.Code)
	}
	f := validForm()
	f.Set("patient_name", "Иванов Пётр")
	f.Del("items[1].name")
	f.Del("items[1].dosage")
	f.Del("items[1].times_per_day")
	f.Del("items[1].days")
	f.Del("items[1].meal")
	f.Del("items[1].times")
	w = postForm(h, "/doctor/prescriptions/"+itoa(id), f, c)
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/doctor/prescriptions/"+itoa(id) {
		t.Fatalf("сохранение: %d %s", w.Code, w.Body.String())
	}
	var name, code2 string
	var n int
	pool.QueryRow(context.Background(), `SELECT patient_name, code, (SELECT count(*) FROM prescription_items WHERE prescription_id = $1) FROM prescriptions WHERE id = $1`, id).Scan(&name, &code2, &n)
	if name != "Иванов Пётр" || code2 != code || n != 1 {
		t.Errorf("после правки: %q %q %d", name, code2, n)
	}
}

// После скана менять нельзя: приёмы уже у пациента.
func TestEditAfterScanRefused(t *testing.T) {
	pool, h := newTestEnv(t)
	c := doctorCookies(t, h, "doctor1")
	id, _ := createRx(t, pool, h, c)
	pool.Exec(context.Background(), `UPDATE prescriptions SET start_date = current_date, claimed_at = now() WHERE id = $1`, id)
	if w := get(h, "/doctor/prescriptions/"+itoa(id)+"/edit", c); w.Code != http.StatusSeeOther {
		t.Errorf("форма правки после скана: %d", w.Code)
	}
	f := validForm()
	f.Set("patient_name", "Другой")
	w := postForm(h, "/doctor/prescriptions/"+itoa(id), f, c)
	if w.Code != http.StatusConflict {
		t.Errorf("правка после скана: %d", w.Code)
	}
	var name string
	pool.QueryRow(context.Background(), `SELECT patient_name FROM prescriptions WHERE id = $1`, id).Scan(&name)
	if name != "Иванов Иван Иванович" {
		t.Errorf("имя изменилось: %q", name)
	}
}

// Отмена: рецепт помечен, публичная страница и список это показывают, править уже нельзя.
func TestCancelPrescription(t *testing.T) {
	pool, h := newTestEnv(t)
	c := doctorCookies(t, h, "doctor1")
	id, code := createRx(t, pool, h, c)
	if w := postForm(h, "/doctor/prescriptions/"+itoa(id)+"/cancel", nil, doctorCookies(t, h, "doctor2")); w.Code != 404 {
		t.Errorf("чужой врач отменил: %d", w.Code)
	}
	w := postForm(h, "/doctor/prescriptions/"+itoa(id)+"/cancel", nil, c)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("отмена: %d", w.Code)
	}
	var cancelled bool
	pool.QueryRow(context.Background(), `SELECT cancelled_at IS NOT NULL FROM prescriptions WHERE id = $1`, id).Scan(&cancelled)
	if !cancelled {
		t.Fatal("не отменён")
	}
	if b := get(h, "/r/"+code, nil).Body.String(); !strings.Contains(b, "Рецепт отменён врачом") || strings.Contains(b, "/app/scan?code=") {
		t.Error("публичная страница не показывает отмену")
	}
	if b := get(h, "/doctor/prescriptions", c).Body.String(); !strings.Contains(b, "Отменён") {
		t.Error("список не показывает отмену")
	}
	if w := postForm(h, "/doctor/prescriptions/"+itoa(id), validForm(), c); w.Code != http.StatusConflict {
		t.Errorf("правка отменённого: %d", w.Code)
	}
}

// «Скопировать в новый»: форма нового рецепта заполнена из старого.
func TestCopyToNew(t *testing.T) {
	pool, h := newTestEnv(t)
	c := doctorCookies(t, h, "doctor1")
	id, _ := createRx(t, pool, h, c)
	b := get(h, "/doctor/?from="+itoa(id), c).Body.String()
	if !strings.Contains(b, `value="Иванов Иван Иванович"`) || !strings.Contains(b, `action="/doctor/prescriptions"`) {
		t.Error("копия не заполнена")
	}
	if b := get(h, "/doctor/?from="+itoa(id), doctorCookies(t, h, "doctor2")).Body.String(); strings.Contains(b, "Иванов Иван") {
		t.Error("чужой рецепт скопирован")
	}
}
