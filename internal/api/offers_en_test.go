package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"erecept/internal/db/dbtest"
	"erecept/internal/i18n"
	"erecept/internal/seed"
)

func offersEnv(t *testing.T) *env {
	t.Helper()
	pool := dbtest.New(t)
	if err := seed.Run(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, pool: pool, h: i18n.Middleware((&API{DB: pool}).Routes())}
}

func getOffers(t *testing.T, e *env, q string) OffersDTO {
	t.Helper()
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, httptest.NewRequest("GET", "/offers?"+q, nil))
	var d OffersDTO
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	return d
}

// Английское приложение: поиск по английскому названию, английские названия, формы и пометка; картинка упаковки.
func TestOffersEnglish(t *testing.T) {
	e := offersEnv(t)
	d := getOffers(t, e, "name=Glucophage&lang=en")
	if d.Medicine == nil || d.Medicine.Name != "Glucophage" || d.Medicine.ActiveSubstance != "metformin" || d.Medicine.Form != "tablets 850 mg, 60 pcs" {
		t.Fatalf("лекарство: %+v", d.Medicine)
	}
	if d.Medicine.Image != "/static/meds/glucophage.png" {
		t.Errorf("картинка: %q", d.Medicine.Image)
	}
	if len(d.Offers) == 0 || !strings.HasPrefix(d.Offers[0].Pharmacy.Address, "Shymkent, ") || !strings.HasPrefix(d.Offers[0].Pharmacy.Name, "Pharmacy «") {
		t.Errorf("аптеки: %+v", d.Offers)
	}
	if len(d.Analogs) == 0 || d.Analogs[0].Medicine.Name != "Metformin" || d.Analogs[0].Medicine.Image != "/static/meds/metformin.png" {
		t.Errorf("аналоги: %+v", d.Analogs)
	}
	if strings.ContainsAny(d.AnalogNote, "абвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
		t.Errorf("пометка: %q", d.AnalogNote)
	}
}

// Русское приложение — как было, но аптеки уже в Шымкенте.
func TestOffersRussianShymkent(t *testing.T) {
	e := offersEnv(t)
	d := getOffers(t, e, "name=Глюкофаж")
	if d.Medicine == nil || d.Medicine.Name != "Глюкофаж" || d.Medicine.Form != "таблетки 850 мг №60" || d.Medicine.Image != "/static/meds/glucophage.png" {
		t.Fatalf("лекарство: %+v", d.Medicine)
	}
	if !strings.HasPrefix(d.Offers[0].Pharmacy.Address, "Шымкент, ") {
		t.Errorf("адрес: %q", d.Offers[0].Pharmacy.Address)
	}
}

func TestFormEN(t *testing.T) {
	for in, want := range map[string]string{
		"таблетки 850 мг №60":         "tablets 850 mg, 60 pcs",
		"капсулы 20 мг №30":           "capsules 20 mg, 30 pcs",
		"сироп 30 мг/5 мл 100 мл":     "syrup 30 mg/5 ml 100 ml",
		"шипучие таблетки 600 мг №10": "effervescent tablets 600 mg, 10 pcs",
		"таблетки 25 ЕД №50":          "tablets 25 U, 50 pcs",
	} {
		if got := formEN(in); got != want {
			t.Errorf("%q: %q, нужно %q", in, got, want)
		}
	}
}
