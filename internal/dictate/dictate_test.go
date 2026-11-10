package dictate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

func fakeClaude(t *testing.T, stopReason, text string, gotBody *[]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

func newParser(url string) *Parser {
	return New("test-key", option.WithBaseURL(url), option.WithMaxRetries(0))
}

func TestParseCleansModelOutput(t *testing.T) {
	var body []byte
	srv := fakeClaude(t, "end_turn", `{
		"patient_name": " Иванова Галина Петровна ",
		"patient_birth": "14.03.1956",
		"comment": "Контроль давления через месяц",
		"items": [
			{"name": "амлодипин", "dosage": "1 таблетка 5 мг", "times_per_day": 1, "times": ["08:00"], "meal": "after", "days": 30},
			{"name": "Бисопролол", "dosage": "1 таблетка 5 мг", "times_per_day": 9, "times": ["8:00", "20:00", "мусор"], "meal": "wrong", "days": 900},
			{"name": "", "dosage": "", "times_per_day": 1, "times": [], "meal": "any", "days": 0}
		]}`, &body)
	d, err := newParser(srv.URL).Parse(context.Background(), "Иванова Галина Петровна, амлодипин…", []string{"Амлодипин", "Бисопролол"})
	if err != nil {
		t.Fatal(err)
	}
	if d.PatientName != "Иванова Галина Петровна" || d.PatientBirth != "14.03.1956" || d.Comment == "" {
		t.Errorf("пациент: %+v", d)
	}
	if len(d.Items) != 2 {
		t.Fatalf("пустая строка должна отброситься, строк %d", len(d.Items))
	}
	a := d.Items[0]
	if a.Name != "Амлодипин" || a.TimesPerDay != 1 || a.Times != "08:00" || a.Meal != "after" || a.Days != 30 {
		t.Errorf("первая строка (название из справочника): %+v", a)
	}
	b := d.Items[1]
	if b.TimesPerDay != 2 || b.Times != "08:00, 20:00" || b.Meal != "any" || b.Days != 365 {
		t.Errorf("вторая строка не почищена: %+v", b)
	}
	// Справочник и текст врача уходят в запрос, текст — как данные в теге.
	var req struct {
		Messages []struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Messages) == 0 || len(req.Messages[0].Content) == 0 {
		t.Fatalf("запрос не разобран: %v", err)
	}
	if u := req.Messages[0].Content[0].Text; !strings.Contains(u, "Бисопролол") || !strings.Contains(u, "<dictation>\nИванова") {
		t.Errorf("в запросе нет справочника или текста: %s", u)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := newParser("http://127.0.0.1:1").Parse(context.Background(), "  ", nil); err == nil {
		t.Error("пустой текст должен давать ошибку")
	}
	srv := fakeClaude(t, "refusal", "", nil)
	if _, err := newParser(srv.URL).Parse(context.Background(), "амлодипин", nil); err == nil {
		t.Error("отказ модели должен давать ошибку")
	}
	srv = fakeClaude(t, "end_turn", "не json", nil)
	if _, err := newParser(srv.URL).Parse(context.Background(), "амлодипин", nil); err == nil {
		t.Error("не-JSON должен давать ошибку")
	}
}

// Английская диктовка: модель просят писать дозировку и комментарий на языке диктовки.
func TestPromptKeepsDictationLanguage(t *testing.T) {
	if !strings.Contains(systemPrompt, "на языке диктовки") || !strings.Contains(systemPrompt, "1 tablet 850 mg") {
		t.Error("в подсказке модели нет правила о языке диктовки")
	}
}
