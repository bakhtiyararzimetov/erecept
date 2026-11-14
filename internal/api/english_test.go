package api

import (
	"context"
	"encoding/json"
	"testing"

	"erecept/internal/db/dbtest"
	"erecept/internal/i18n"
)

// Приложение на английском (?lang=en): английское название из справочника, подпись про еду и памятка.
func TestPrescriptionDTOEnglish(t *testing.T) {
	pool := dbtest.New(t)
	e := &env{t: t, pool: pool, h: i18n.Middleware((&API{DB: pool}).Routes())}
	ctx := context.Background()
	itemID := e.newPrescription("english1")
	var medID int64
	if err := pool.QueryRow(ctx, `INSERT INTO medicines (name, active_substance, name_en, substance_en) VALUES ('Парацетамол', 'парацетамол', 'Paracetamol', 'paracetamol') RETURNING id`).Scan(&medID); err != nil {
		t.Fatal(err)
	}
	pool.Exec(ctx, `UPDATE prescription_items SET medicine_id = $1 WHERE id = $2`, medID, itemID)
	pool.Exec(ctx, `INSERT INTO medicine_explanations (medicine_id, data, data_en) VALUES ($1, '{"purpose":"Снижает температуру."}', '{"purpose":"Lowers fever."}')`, medID)

	w := e.do("POST", "/prescriptions/english1/claim?lang=en", "secret-english-0123456789", nil)
	if w.Code != 200 {
		t.Fatalf("claim: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Prescription struct {
			Items []struct {
				MedicineName string          `json:"medicine_name"`
				Image        string          `json:"image"`
				MealLabel    string          `json:"meal_label"`
				Explanation  json.RawMessage `json:"explanation"`
			} `json:"items"`
		} `json:"prescription"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	it := resp.Prescription.Items[0]
	if it.Image != "/static/meds/en/paracetamol.png" {
		t.Errorf("картинка: %q", it.Image)
	}
	if it.MedicineName != "Paracetamol" || it.MealLabel != "after meals" || string(it.Explanation) != `{"purpose": "Lowers fever."}` && string(it.Explanation) != `{"purpose":"Lowers fever."}` {
		t.Errorf("%+v %s", it, it.Explanation)
	}
	w = e.do("GET", "/me/prescriptions", "secret-english-0123456789", nil)
	if json.Unmarshal(w.Body.Bytes(), &[]any{}) == nil && !json.Valid(w.Body.Bytes()) {
		t.Error("не JSON")
	}
}
