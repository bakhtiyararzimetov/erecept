package boxcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

func fakeClaude(t *testing.T, text string, gotBody *[]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if gotBody != nil {
			*gotBody = b
		}
		resp := map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"content":       []map[string]any{{"type": "text", "text": text}},
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 10, "output_tokens": 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newChecker(url string) *Checker {
	return New("test-key", option.WithBaseURL(url), option.WithMaxRetries(0))
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

var glucophage = []Item{{Name: "Глюкофаж", Substance: "метформин", Dosage: "1 таблетка 850 мг"}}

// Другая марка с тем же веществом и дозой — аналог.
func TestCheckAnalog(t *testing.T) {
	var body []byte
	srv := fakeClaude(t, `{"box":{"brand":"Сиофор","substance":"метформин","strength":"850 мг","form":"таблетки"},"verdict":"analog","item":"глюкофаж"}`, &body)
	res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), glucophage, RU)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "analog" || res.Item != "Глюкофаж" || res.Box.Brand != "Сиофор" {
		t.Errorf("результат: %+v", res)
	}
	if !strings.Contains(res.Message, "Аналог") || !strings.Contains(res.Message, "метформин") {
		t.Errorf("сообщение: %q", res.Message)
	}
	var req struct {
		Messages []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatal(err)
	}
	var hasImage, hasRx bool
	for _, c := range req.Messages[0].Content {
		hasImage = hasImage || c.Type == "image"
		hasRx = hasRx || strings.Contains(c.Text, "Глюкофаж") && strings.Contains(c.Text, "метформин")
	}
	if !hasImage || !hasRx {
		t.Errorf("в запросе нет фото или рецепта: %s", body)
	}
}

// Модель назвала лекарство не из рецепта — ответу не верим.
func TestCheckDowngradesUnknownItem(t *testing.T) {
	srv := fakeClaude(t, `{"box":{"brand":"Аспирин","substance":"ацетилсалициловая кислота","strength":"500 мг","form":"таблетки"},"verdict":"match","item":"Аспирин"}`, nil)
	res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), glucophage, RU)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "unreadable" {
		t.Errorf("ожидали unreadable, получили %+v", res)
	}
}

func TestCheckUnknownVerdict(t *testing.T) {
	srv := fakeClaude(t, `{"box":{"brand":"","substance":"","strength":"","form":""},"verdict":"maybe","item":""}`, nil)
	res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), glucophage, RU)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "unreadable" || !strings.Contains(res.Message, "Не удалось прочитать") {
		t.Errorf("результат: %+v", res)
	}
}

// Ни один ответ не разрешает принимать лекарство; все несовпадения отправляют к фармацевту.
func TestCheckMessagesSafe(t *testing.T) {
	box := Box{Brand: "Глюкофаж Лонг", Substance: "метформин", Strength: "750 мг", Form: "таблетки пролонгированного действия"}
	for _, v := range verdicts {
		m := message(v, box, glucophage[0])
		if m == "" || strings.Contains(strings.ToLower(m), "можно принимать") {
			t.Errorf("%s: %q", v, m)
		}
		if v != "match" && v != "unreadable" && !strings.Contains(m, "фармацевт") {
			t.Errorf("%s без фармацевта: %q", v, m)
		}
	}
	if len(verdicts) != 7 {
		t.Errorf("вердиктов %d, нужно 7", len(verdicts))
	}
}

func TestCheckBadImage(t *testing.T) {
	c := newChecker("http://127.0.0.1:1")
	for _, img := range [][]byte{nil, []byte("hello")} {
		if _, err := c.Check(context.Background(), img, glucophage, RU); !errors.Is(err, ErrImage) {
			t.Errorf("%q: ошибка %v", img, err)
		}
	}
}

// Модель говорит «аналог», но вещество на упаковке другое — ✅ показывать нельзя.
func TestCheckAnalogNeedsSameSubstance(t *testing.T) {
	srv := fakeClaude(t, `{"box":{"brand":"Аспирин","substance":"ацетилсалициловая кислота","strength":"850 мг","form":"таблетки"},"verdict":"analog","item":"Глюкофаж"}`, nil)
	res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), glucophage, RU)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "unreadable" {
		t.Errorf("ожидали unreadable, получили %+v", res)
	}
}

// Лекарство вне справочника: вещество рецепта неизвестно, поэтому «аналог» — только с фармацевтом.
func TestCheckAnalogWithoutKnownSubstanceWarns(t *testing.T) {
	srv := fakeClaude(t, `{"box":{"brand":"Сиофор","substance":"метформин","strength":"850 мг","form":"таблетки"},"verdict":"analog","item":"Глюкофаж"}`, nil)
	items := []Item{{Name: "Глюкофаж", Dosage: "1 таблетка 850 мг"}}
	res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), items, RU)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "analog_unverified" || !strings.Contains(res.Message, "фармацевт") || strings.Contains(res.Message, "✅") {
		t.Errorf("результат: %+v", res)
	}
}

// Пролонгированная форма при обычном препарате в рецепте — всегда «форма отличается».
func TestCheckExtendedReleaseIsFormDiff(t *testing.T) {
	for _, verdict := range []string{"match", "analog"} {
		srv := fakeClaude(t, `{"box":{"brand":"Глюкофаж Лонг","substance":"метформин","strength":"750 мг","form":"таблетки"},"verdict":"`+verdict+`","item":"Глюкофаж"}`, nil)
		res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), glucophage, RU)
		if err != nil {
			t.Fatal(err)
		}
		if res.Verdict != "form_diff" {
			t.Errorf("%s: ожидали form_diff, получили %+v", verdict, res)
		}
	}
}

// Английские ответы: те же правила — не «можно принимать», все несовпадения — к фармацевту.
func TestCheckMessagesSafeEN(t *testing.T) {
	box := Box{Brand: "Metfora", Substance: "метформин", SubstanceEN: "metformin", Strength: "850 mg", Form: "tablets"}
	item := Item{Name: "Глюкофаж", NameEN: "Glucophage", Substance: "метформин", SubstanceEN: "metformin", Dosage: "1 tablet 850 mg"}
	for _, v := range append(append([]string{}, verdicts...), "analog_unverified") {
		m := messageIn(EN, v, box, item)
		low := strings.ToLower(m)
		if m == "" || strings.Contains(low, "safe to take") || strings.ContainsAny(m, "абвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
			t.Errorf("%s: %q", v, m)
		}
		if v != "match" && v != "unreadable" && !strings.Contains(low, "pharmacist") {
			t.Errorf("%s без pharmacist: %q", v, m)
		}
	}
	if m := messageIn(EN, "analog", box, item); !strings.Contains(m, "metformin") || !strings.Contains(m, "Glucophage") {
		t.Errorf("analog EN: %q", m)
	}
}

func TestCheckEnglishResult(t *testing.T) {
	srv := fakeClaude(t, `{"box":{"brand":"Metfora","substance":"метформин","substance_en":"metformin","strength":"850 мг","form":"таблетки"},"verdict":"analog","item":"Глюкофаж"}`, nil)
	items := []Item{{Name: "Глюкофаж", NameEN: "Glucophage", Substance: "метформин", SubstanceEN: "metformin", Dosage: "1 таблетка 850 мг"}}
	res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), items, EN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "analog" || !strings.HasPrefix(res.Message, "✅ Analog") || res.Box.SubstanceEN != "metformin" {
		t.Errorf("%+v", res)
	}
}

// В рецепте для модели строка «Глюкофаж (Glucophage)» — любой из вариантов названия считается той же строкой.
func TestCheckItemNameVariants(t *testing.T) {
	items := []Item{{Name: "Глюкофаж", NameEN: "Glucophage", Substance: "метформин", SubstanceEN: "metformin", Dosage: "1 таблетка 850 мг"}}
	for _, name := range []string{"Глюкофаж (Glucophage)", "Glucophage", "глюкофаж"} {
		srv := fakeClaude(t, `{"box":{"brand":"Сиофор","substance":"метформин","substance_en":"metformin","strength":"850 мг","form":"таблетки"},"verdict":"analog","item":"`+name+`"}`, nil)
		res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), items, RU)
		if err != nil {
			t.Fatal(err)
		}
		if res.Verdict != "analog" || res.Item != "Глюкофаж" {
			t.Errorf("%q: %+v", name, res)
		}
	}
}

// Английский ответ показывает форму и дозу с упаковки по-английски.
func TestCheckEnglishFormAndStrength(t *testing.T) {
	srv := fakeClaude(t, `{"box":{"brand":"Глюкофаж Лонг","substance":"метформин","substance_en":"metformin","strength":"750 мг","strength_en":"750 mg","form":"таблетки пролонгированного действия","form_en":"extended-release tablets"},"verdict":"analog","item":"Глюкофаж"}`, nil)
	res, err := newChecker(srv.URL).Check(context.Background(), pngBytes(t), []Item{{Name: "Глюкофаж", NameEN: "Glucophage", Substance: "метформин", SubstanceEN: "metformin", Dosage: "1 tablet 850 mg"}}, EN)
	if err != nil {
		t.Fatal(err)
	}
	if res.Verdict != "form_diff" || !strings.Contains(res.Message, "extended-release tablets") || strings.ContainsAny(res.Message, "абвгдеёжзийклмнопрстуфхцчшщъыьэюя") {
		t.Errorf("%+v", res)
	}
}

// Каждое обязательное поле схемы описано в properties (иначе API отклонит запрос).
func TestSchemaRequiredHaveProperties(t *testing.T) {
	box := outputSchema["properties"].(map[string]any)["box"].(map[string]any)
	props := box["properties"].(map[string]any)
	for _, f := range box["required"].([]string) {
		if _, ok := props[f]; !ok {
			t.Errorf("поле %q есть в required, но нет в properties", f)
		}
	}
}
