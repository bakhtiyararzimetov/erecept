package explain

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/jackc/pgx/v5/pgxpool"

	"erecept/internal/db/dbtest"
	"erecept/internal/leaflet"
)

func TestIsHighRisk(t *testing.T) {
	cases := []struct {
		mnn, atc string
		want     bool
	}{
		{"Варфарин", "", true},
		{"инсулин гларгин", "", true},
		{"", "B01AA03", true},
		{"Warfarin", "", true},
		{"Этинилэстрадиол + левоноргестрел", "", true},
		{"вальпроевая кислота", "", true},
		{"парацетамол", "", false},
		{"ибупрофен", "M01AE01", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := IsHighRisk(c.mnn, c.atc); got != c.want {
			t.Errorf("IsHighRisk(%q, %q) = %v, ожидалось %v", c.mnn, c.atc, got, c.want)
		}
	}
}

const modelJSON = `{
  "purpose": "Снижает температуру и уменьшает боль.",
  "how_to_take": "По 1 таблетке после еды.",
  "do_not_take_if": ["Аллергия на препарат", "Тяжёлые болезни печени"],
  "missed_dose": "Выпейте, как вспомните.",
  "if_stopped_early": "Можно прекратить, когда симптомы прошли.",
  "urgent_signs": ["Сыпь, отёк лица — вызовите 103"]
}`

// fakeClaude отвечает как Messages API: stop_reason и текстовый блок с памяткой.
func fakeClaude(t *testing.T, stopReason, text string, calls *atomic.Int32, gotBody *[]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("неожиданный путь %s", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		if gotBody != nil {
			*gotBody = b
		}
		resp := map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"content":       []map[string]any{{"type": "text", "text": text}},
			"stop_reason":   stopReason,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 10, "output_tokens": 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newService(pool *pgxpool.Pool, lc *leaflet.Client, claudeURL string) *Service {
	return New(pool, lc, "test-key", option.WithBaseURL(claudeURL), option.WithMaxRetries(0))
}

func TestGenerateParsesAndOverridesHighRisk(t *testing.T) {
	var body []byte
	srv := fakeClaude(t, "end_turn", modelJSON, nil, &body)
	s := newService(nil, nil, srv.URL)
	e := leaflet.Entry{RegisterID: 1, TradeName: "Парацетамол", RegNumber: "РК-ЛС-5№000001", MNN: "парацетамол"}

	ex, err := s.Generate(context.Background(), "Парацетамол", "ТЕКСТ ИНСТРУКЦИИ", e)
	if err != nil {
		t.Fatal(err)
	}
	if ex.Purpose != "Снижает температуру и уменьшает боль." || len(ex.DoNotTakeIf) != 2 || len(ex.UrgentSigns) != 1 ||
		ex.MissedDose != "Выпейте, как вспомните." || ex.HighRisk {
		t.Fatalf("поля не разобраны: %+v", ex)
	}
	if ex.Source.RegNumber != "РК-ЛС-5№000001" || ex.Source.URL == "" || ex.Source.Title == "" {
		t.Errorf("источник: %+v", ex.Source)
	}
	if ex.Model != "claude-opus-5-5" || ex.GeneratedAt.IsZero() {
		t.Errorf("model/generated_at: %q %v", ex.Model, ex.GeneratedAt)
	}

	// Запрос: модель, effort, structured output, инструкция в теге.
	var req struct {
		Model        string `json:"model"`
		MaxTokens    int    `json:"max_tokens"`
		OutputConfig struct {
			Effort string `json:"effort"`
			Format struct {
				Type   string         `json:"type"`
				Schema map[string]any `json:"schema"`
			} `json:"format"`
		} `json:"output_config"`
		Thinking json.RawMessage `json:"thinking"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	if req.Model != "claude-opus-5-5" || req.MaxTokens != 4000 || req.OutputConfig.Effort != "medium" ||
		req.OutputConfig.Format.Type != "json_schema" || req.OutputConfig.Format.Schema == nil {
		t.Errorf("параметры запроса: %s", body)
	}
	if req.Thinking != nil {
		t.Errorf("thinking не должен передаваться: %s", req.Thinking)
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != "user" ||
		!strings.Contains(req.Messages[0].Content[0].Text, "<instruction>\nТЕКСТ ИНСТРУКЦИИ\n</instruction>") {
		t.Errorf("сообщение: %s", body)
	}

	// Варфарин: missed_dose всегда «позвоните врачу», что бы ни ответила модель.
	ex, err = s.Generate(context.Background(), "Варфарин Никомед", "ТЕКСТ", leaflet.Entry{RegNumber: "X", MNN: "Варфарин"})
	if err != nil {
		t.Fatal(err)
	}
	if !ex.HighRisk || ex.MissedDose != HighRiskMissedDose {
		t.Fatalf("высокий риск не применён: %v %q", ex.HighRisk, ex.MissedDose)
	}
}

func TestGenerateRefusal(t *testing.T) {
	srv := fakeClaude(t, "refusal", "", nil, nil)
	s := newService(nil, nil, srv.URL)
	if _, err := s.Generate(context.Background(), "Парацетамол", "Т", leaflet.Entry{}); err == nil {
		t.Fatal("отказ модели должен давать ошибку")
	}
}

func TestGenerateBadOutput(t *testing.T) {
	for name, tc := range map[string]struct{ stop, text string }{
		"max_tokens": {"max_tokens", `{"purpose": "Сни`},
		"not_json":   {"end_turn", "не JSON"},
		"empty":      {"end_turn", `{"purpose":"","how_to_take":"","do_not_take_if":[],"missed_dose":"","if_stopped_early":"","urgent_signs":[]}`},
	} {
		t.Run(name, func(t *testing.T) {
			srv := fakeClaude(t, tc.stop, tc.text, nil, nil)
			s := newService(nil, nil, srv.URL)
			if _, err := s.Generate(context.Background(), "Парацетамол", "Т", leaflet.Entry{}); err == nil {
				t.Fatal("ожидалась ошибка")
			}
		})
	}
}

func makeDocx(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="x"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	zw.Close()
	return buf.Bytes()
}

// fakeRegistry: /list отдаёт одну запись (или пустой список), инструкция — DOCX.
func fakeRegistry(t *testing.T, found bool, calls *atomic.Int32) *leaflet.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/list":
			if !found {
				w.Write([]byte(`[]`))
				return
			}
			w.Write([]byte(`[{"id":42,"drugTradeName":"Парацетамол","regNumber":"РК-ЛС-5№000042","mnn":"парацетамол"}]`))
		case "/getRegisterInstructionFile":
			w.Write(makeDocx(t, "Показания: боль и температура."))
		default:
			t.Errorf("неожиданный путь %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return &leaflet.Client{BaseURL: srv.URL, HTTP: srv.Client()}
}

func insertMedicine(t *testing.T, pool *pgxpool.Pool, name, substance string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO medicines (name, active_substance) VALUES ($1, $2) RETURNING id`,
		name, substance).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEnsureStoresAndSkips(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	var regCalls, claudeCalls atomic.Int32
	lc := fakeRegistry(t, true, &regCalls)
	srv := fakeClaude(t, "end_turn", modelJSON, &claudeCalls, nil)
	s := newService(pool, lc, srv.URL)
	id := insertMedicine(t, pool, "Парацетамол", "парацетамол")

	if err := s.Ensure(ctx, id); err != nil {
		t.Fatal(err)
	}
	var data []byte
	if err := pool.QueryRow(ctx, `SELECT data FROM medicine_explanations WHERE medicine_id = $1`, id).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var ex Explanation
	if err := json.Unmarshal(data, &ex); err != nil || ex.Purpose == "" || ex.Source.RegNumber != "РК-ЛС-5№000042" {
		t.Fatalf("сохранено не то: %s (%v)", data, err)
	}
	reg, cl := regCalls.Load(), claudeCalls.Load()
	if reg != 2 || cl != 1 {
		t.Fatalf("вызовы: реестр %d, Claude %d", reg, cl)
	}
	if err := s.Ensure(ctx, id); err != nil {
		t.Fatal(err)
	}
	if regCalls.Load() != reg || claudeCalls.Load() != cl {
		t.Fatal("повторный Ensure не должен ходить в сеть")
	}
}

func TestEnsureHighRiskBySubstance(t *testing.T) {
	pool := dbtest.New(t)
	var regCalls atomic.Int32
	lc := fakeRegistry(t, true, &regCalls) // реестр отдаёт МНН «парацетамол», но в справочнике — варфарин
	srv := fakeClaude(t, "end_turn", modelJSON, nil, nil)
	s := newService(pool, lc, srv.URL)
	id := insertMedicine(t, pool, "Варфарин", "варфарин")
	if err := s.Ensure(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	var ex Explanation
	if err := pool.QueryRow(context.Background(), `SELECT data FROM medicine_explanations WHERE medicine_id = $1`, id).Scan(&ex); err != nil {
		t.Fatal(err)
	}
	if !ex.HighRisk || ex.MissedDose != HighRiskMissedDose {
		t.Fatalf("высокий риск по МНН справочника не применён: %+v", ex)
	}
}

func TestEnsureNotFoundRecordsFailure(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	var regCalls, claudeCalls atomic.Int32
	lc := fakeRegistry(t, false, &regCalls)
	srv := fakeClaude(t, "end_turn", modelJSON, &claudeCalls, nil)
	s := newService(pool, lc, srv.URL)
	id := insertMedicine(t, pool, "Нет такого", "")

	if err := s.Ensure(ctx, id); err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if claudeCalls.Load() != 0 {
		t.Error("без инструкции Claude не вызывается")
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM medicine_explanations`).Scan(&n)
	if n != 0 {
		t.Error("памятка не должна сохраниться")
	}
	var reason string
	if err := pool.QueryRow(ctx, `SELECT reason FROM medicine_explanation_failures WHERE medicine_id = $1`, id).Scan(&reason); err != nil || reason == "" {
		t.Fatalf("неудача не записана: %q %v", reason, err)
	}
	// Свежая неудача: фоновый проход это лекарство пропускает.
	todo, err := s.pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range todo {
		if m.id == id {
			t.Fatal("лекарство со свежей неудачей не должно попадать в очередь")
		}
	}
	// Через 24 ч — снова в очереди.
	pool.Exec(ctx, `UPDATE medicine_explanation_failures SET failed_at = now() - interval '25 hours'`)
	todo, _ = s.pending(ctx)
	if len(todo) != 1 || todo[0].id != id {
		t.Fatalf("после 24 ч ожидалось лекарство в очереди: %+v", todo)
	}
}

func TestBackfillStopsOnAuthError(t *testing.T) {
	pool := dbtest.New(t)
	var regCalls, claudeCalls atomic.Int32
	lc := fakeRegistry(t, true, &regCalls)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claudeCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	t.Cleanup(srv.Close)
	s := newService(pool, lc, srv.URL)
	insertMedicine(t, pool, "Парацетамол", "парацетамол")
	insertMedicine(t, pool, "Нурофен", "ибупрофен")
	s.backfill(context.Background())
	if claudeCalls.Load() != 1 {
		t.Fatalf("при неверном ключе проход должен остановиться после первого лекарства, вызовов: %d", claudeCalls.Load())
	}
}
