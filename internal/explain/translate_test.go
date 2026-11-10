package explain

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"erecept/internal/db/dbtest"
)

const modelJSONEN = `{
  "purpose": "Lowers fever and reduces pain.",
  "how_to_take": "1 tablet after meals. Your doctor set the dose — take it as prescribed.",
  "do_not_take_if": ["Allergy to the medicine"],
  "missed_dose": "Take it when you remember.",
  "if_stopped_early": "You can stop when the symptoms are gone.",
  "urgent_signs": ["Rash, swollen face — call 103"]
}`

func TestTranslateKeepsSourceAndMeta(t *testing.T) {
	var body []byte
	srv := fakeClaude(t, "end_turn", modelJSONEN, nil, &body)
	s := newService(nil, nil, srv.URL)
	ru := Explanation{Purpose: "Снижает температуру.", HowToTake: "После еды.", MissedDose: "Выпейте.", IfStoppedEarly: "Можно.", HighRisk: true,
		Source: Source{Title: sourceTitle, RegNumber: "РК-ЛС-5№1"}, Model: "claude-opus-5-5"}
	en, err := s.Translate(context.Background(), ru)
	if err != nil {
		t.Fatal(err)
	}
	if en.Purpose != "Lowers fever and reduces pain." || len(en.UrgentSigns) != 1 || en.Source.RegNumber != "РК-ЛС-5№1" || !en.HighRisk {
		t.Errorf("%+v", en)
	}
	if !strings.Contains(string(body), "Снижает температуру") {
		t.Errorf("в запросе нет русской памятки: %s", body)
	}
	if en.MissedDose != HighRiskMissedDoseEN {
		t.Errorf("высокий риск: %q", en.MissedDose)
	}
}

func TestBackfillEnglish(t *testing.T) {
	pool := dbtest.New(t)
	var calls atomic.Int32
	srv := fakeClaude(t, "end_turn", modelJSONEN, &calls, nil)
	s := newService(pool, nil, srv.URL)
	id := insertMedicine(t, pool, "Парацетамол", "парацетамол")
	data, _ := json.Marshal(Explanation{Purpose: "Снижает температуру.", HowToTake: "После еды.", MissedDose: "Выпейте.", IfStoppedEarly: "Можно."})
	if _, err := pool.Exec(context.Background(), `INSERT INTO medicine_explanations (medicine_id, data) VALUES ($1, $2)`, id, data); err != nil {
		t.Fatal(err)
	}
	s.backfillEN(context.Background())
	s.backfillEN(context.Background()) // второй проход ничего не переводит заново
	if calls.Load() != 1 {
		t.Errorf("вызовов Claude: %d", calls.Load())
	}
	var raw []byte
	pool.QueryRow(context.Background(), `SELECT data_en FROM medicine_explanations WHERE medicine_id = $1`, id).Scan(&raw)
	if !strings.Contains(string(raw), "Lowers fever") {
		t.Errorf("data_en: %s", raw)
	}
}

// Плохой перевод не повторяется каждый час: следующая попытка — через сутки.
func TestBackfillEnglishRemembersFailure(t *testing.T) {
	pool := dbtest.New(t)
	var calls atomic.Int32
	srv := fakeClaude(t, "end_turn", `not json`, &calls, nil)
	s := newService(pool, nil, srv.URL)
	id := insertMedicine(t, pool, "Парацетамол", "парацетамол")
	pool.Exec(context.Background(), `INSERT INTO medicine_explanations (medicine_id, data) VALUES ($1, '{"purpose":"Снижает температуру."}')`, id)
	s.backfillEN(context.Background())
	s.backfillEN(context.Background())
	if calls.Load() != 1 {
		t.Errorf("после неудачи повтор должен ждать, вызовов: %d", calls.Load())
	}
}
