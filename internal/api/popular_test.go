package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// Популярные лекарства для пустой вкладки «Аптеки»: то, что есть в наличии в большинстве аптек, с ценой «от».
func TestPopularMedicines(t *testing.T) {
	e := offersEnv(t)
	get := func(q string) []AnalogDTO {
		w := httptest.NewRecorder()
		e.h.ServeHTTP(w, httptest.NewRequest("GET", "/medicines/popular"+q, nil))
		var d []AnalogDTO
		if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		return d
	}
	ru := get("")
	if len(ru) < 8 || len(ru) > 12 {
		t.Fatalf("сколько: %d", len(ru))
	}
	for i, a := range ru {
		if a.MinPrice <= 0 || a.PharmaciesCount < 1 || a.Medicine.Image == "" {
			t.Errorf("%d: %+v", i, a)
		}
		if i > 0 && a.PharmaciesCount > ru[i-1].PharmaciesCount {
			t.Errorf("не по убыванию аптек: %+v", ru)
		}
	}
	en := get("?lang=en")
	for _, a := range en {
		if strings.ContainsAny(a.Medicine.Name+a.Medicine.Form, "абвгдеёжзийклмнопрстуфхцчшщъыьэюя") || !strings.HasPrefix(a.Medicine.Image, "/static/meds/en/") {
			t.Errorf("en: %+v", a)
		}
	}
}
