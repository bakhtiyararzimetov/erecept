// Package explain: короткая памятка о лекарстве для пациента. Claude пересказывает
// только официальную инструкцию НЦЭЛС; памятка один раз создаётся фоном и хранится в Postgres.
package explain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/jackc/pgx/v5/pgxpool"

	"erecept/internal/leaflet"
)

const (
	defaultModel  = "claude-opus-5-5"
	maxTokens     = 4000
	sourceTitle   = "Инструкция по медицинскому применению (НЦЭЛС)"
	notInText     = "В инструкции не указано, спросите врача или фармацевта."
	maxListItems  = 4
	ensureTimeout = 5 * time.Minute
	retryAfter    = 24 * time.Hour
	backfillEvery = time.Hour
	backfillPause = 2 * time.Second
)

// errNoAccess: неверный ключ или нет доступа — фоновый проход останавливается.
var errNoAccess = errors.New("нет доступа к Claude API, проверьте ключ")

type Source struct {
	Title     string `json:"title"`
	RegNumber string `json:"reg_number"`
	URL       string `json:"url"`
}

// Explanation — памятка; хранится в medicine_explanations.data и отдаётся приложению как есть.
type Explanation struct {
	Purpose        string    `json:"purpose"`
	HowToTake      string    `json:"how_to_take"`
	DoNotTakeIf    []string  `json:"do_not_take_if"`
	MissedDose     string    `json:"missed_dose"`
	IfStoppedEarly string    `json:"if_stopped_early"`
	UrgentSigns    []string  `json:"urgent_signs"`
	HighRisk       bool      `json:"high_risk"`
	Source         Source    `json:"source"`
	Model          string    `json:"model"`
	GeneratedAt    time.Time `json:"generated_at"`
}

type Service struct {
	DB      *pgxpool.Pool
	Leaflet *leaflet.Client
	Claude  anthropic.Client
	Model   string
}

// New: apiKey — ключ Claude API; opts — для тестов (option.WithBaseURL).
func New(pool *pgxpool.Pool, lc *leaflet.Client, apiKey string, opts ...option.RequestOption) *Service {
	opts = append([]option.RequestOption{option.WithAPIKey(apiKey)}, opts...)
	return &Service{DB: pool, Leaflet: lc, Claude: anthropic.NewClient(opts...), Model: defaultModel}
}

const systemPrompt = `Ты помогаешь пожилым пациентам в Казахстане понять официальную инструкцию к лекарству.
Перескажи ТОЛЬКО то, что написано в инструкции из сообщения пользователя (тег <instruction>). Не добавляй ничего от себя и из общих знаний.

Правила:
- Пиши по-русски, коротко и простыми словами, как для пожилого человека без медицинского образования. Без латыни и сложных терминов: если термин нужен, объясни его просто.
- Каждое текстовое поле — не больше 2 коротких предложений. В списках — не больше 4 пунктов, самые важные.
- Не придумывай дозировки, сроки и цифры: бери их только из инструкции. Если в инструкции дозировка зависит от возраста или болезни, напиши дозу для взрослых и что точную дозу назначает врач.
- Если нужной информации в инструкции нет, напиши в поле ровно: «` + notInText + `»
- Начальная часть текста (приказы, регистрационные данные) — служебная, её не пересказывай.

Поля:
- purpose: зачем это лекарство (показания).
- how_to_take: как принимать (способ применения и дозы для взрослых, связь с едой).
- do_not_take_if: главные противопоказания — кому нельзя.
- missed_dose: что делать, если пропустили приём.
- if_stopped_early: что будет или что делать, если прекратить приём раньше срока.
- urgent_signs: опасные признаки, при которых нужно прекратить приём и срочно обратиться к врачу или вызвать скорую (103).`

// outputSchema — JSON-схема ответа модели. high_risk, source, model и generated_at ставит код.
var outputSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"purpose":          map[string]any{"type": "string"},
		"how_to_take":      map[string]any{"type": "string"},
		"do_not_take_if":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		"missed_dose":      map[string]any{"type": "string"},
		"if_stopped_early": map[string]any{"type": "string"},
		"urgent_signs":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	},
	"required":             []string{"purpose", "how_to_take", "do_not_take_if", "missed_dose", "if_stopped_early", "urgent_signs"},
	"additionalProperties": false,
}

type modelOutput struct {
	Purpose        string   `json:"purpose"`
	HowToTake      string   `json:"how_to_take"`
	DoNotTakeIf    []string `json:"do_not_take_if"`
	MissedDose     string   `json:"missed_dose"`
	IfStoppedEarly string   `json:"if_stopped_early"`
	UrgentSigns    []string `json:"urgent_signs"`
}

// Generate — один запрос к Claude: памятка по тексту инструкции.
func (s *Service) Generate(ctx context.Context, medicineName, instructionText string, e leaflet.Entry) (Explanation, error) {
	user := "Лекарство: " + medicineName + "\n\n<instruction>\n" + instructionText + "\n</instruction>\n\n" +
		"Составь памятку по этой инструкции."
	resp, err := s.Claude.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(s.Model),
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(user))},
		OutputConfig: anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffortMedium,
			Format: anthropic.JSONOutputFormatParam{Schema: outputSchema},
		},
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			if apiErr.StatusCode == 401 || apiErr.StatusCode == 403 {
				return Explanation{}, errNoAccess
			}
			return Explanation{}, fmt.Errorf("Claude API ответил %d", apiErr.StatusCode)
		}
		return Explanation{}, fmt.Errorf("Claude API недоступен: %w", err)
	}
	switch resp.StopReason {
	case anthropic.StopReasonRefusal:
		return Explanation{}, errors.New("модель отказалась отвечать")
	case anthropic.StopReasonMaxTokens:
		return Explanation{}, errors.New("ответ модели обрезан")
	}
	var text strings.Builder
	for _, b := range resp.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			text.WriteString(t.Text)
		}
	}
	var out modelOutput
	if err := json.Unmarshal([]byte(text.String()), &out); err != nil {
		return Explanation{}, errors.New("модель вернула не JSON")
	}
	ex := Explanation{
		Purpose:        strings.TrimSpace(out.Purpose),
		HowToTake:      strings.TrimSpace(out.HowToTake),
		DoNotTakeIf:    cleanList(out.DoNotTakeIf),
		MissedDose:     strings.TrimSpace(out.MissedDose),
		IfStoppedEarly: strings.TrimSpace(out.IfStoppedEarly),
		UrgentSigns:    cleanList(out.UrgentSigns),
		Source:         Source{Title: sourceTitle, RegNumber: e.RegNumber, URL: leaflet.SourceURL(e)},
		Model:          s.Model,
		GeneratedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if ex.Purpose == "" || ex.HowToTake == "" || ex.MissedDose == "" || ex.IfStoppedEarly == "" {
		return Explanation{}, errors.New("в ответе модели пустые поля")
	}
	if IsHighRisk(e.MNN, "") || IsHighRisk(medicineName, "") {
		ex.markHighRisk()
	}
	return ex, nil
}

func (ex *Explanation) markHighRisk() {
	ex.HighRisk = true
	ex.MissedDose = HighRiskMissedDose
}

// cleanList убирает пустые пункты и оставляет не больше maxListItems.
func cleanList(in []string) []string {
	out := []string{}
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && len(out) < maxListItems {
			out = append(out, s)
		}
	}
	return out
}

// Ensure создаёт памятку, если её ещё нет. Неудача записывается в medicine_explanation_failures.
func (s *Service) Ensure(ctx context.Context, medicineID int64) error {
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM medicine_explanations WHERE medicine_id = $1)`, medicineID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	var name, substance string
	if err := s.DB.QueryRow(ctx, `SELECT name, active_substance FROM medicines WHERE id = $1`, medicineID).Scan(&name, &substance); err != nil {
		return err
	}
	ex, err := s.build(ctx, name, substance)
	if err != nil {
		reason := err.Error()
		if r := []rune(reason); len(r) > 300 {
			reason = string(r[:300])
		}
		_, dbErr := s.DB.Exec(ctx, `INSERT INTO medicine_explanation_failures (medicine_id, reason) VALUES ($1, $2)
			ON CONFLICT (medicine_id) DO UPDATE SET reason = EXCLUDED.reason, failed_at = now()`, medicineID, reason)
		return errors.Join(err, dbErr)
	}
	data, err := json.Marshal(ex)
	if err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, `INSERT INTO medicine_explanations (medicine_id, data) VALUES ($1, $2)
		ON CONFLICT (medicine_id) DO NOTHING`, medicineID, data); err != nil {
		return err
	}
	_, err = s.DB.Exec(ctx, `DELETE FROM medicine_explanation_failures WHERE medicine_id = $1`, medicineID)
	return err
}

// build: реестр → инструкция → Claude. Высокий риск проверяется и по МНН из справочника.
func (s *Service) build(ctx context.Context, name, substance string) (Explanation, error) {
	ctx, cancel := context.WithTimeout(ctx, ensureTimeout)
	defer cancel()
	e, err := s.Leaflet.Find(ctx, name)
	if err != nil {
		return Explanation{}, err
	}
	text, err := s.Leaflet.Instruction(ctx, e.RegisterID)
	if err != nil {
		return Explanation{}, err
	}
	ex, err := s.Generate(ctx, name, text, e)
	if err != nil {
		return Explanation{}, err
	}
	if IsHighRisk(substance, "") {
		ex.markHighRisk()
	}
	return ex, nil
}

type medicine struct {
	id   int64
	name string
}

// pending — лекарства без памятки и без неудачи за последние 24 ч.
func (s *Service) pending(ctx context.Context) ([]medicine, error) {
	rows, err := s.DB.Query(ctx, `SELECT m.id, m.name FROM medicines m
		WHERE NOT EXISTS (SELECT 1 FROM medicine_explanations e WHERE e.medicine_id = m.id)
		  AND NOT EXISTS (SELECT 1 FROM medicine_explanation_failures f
		                  WHERE f.medicine_id = m.id AND f.failed_at > now() - $1::interval)
		ORDER BY m.id`, fmt.Sprintf("%d seconds", int(retryAfter.Seconds())))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []medicine
	for rows.Next() {
		var m medicine
		if err := rows.Scan(&m.id, &m.name); err != nil {
			return nil, err
		}
		list = append(list, m)
	}
	return list, rows.Err()
}

// BackfillLoop при старте и раз в час создаёт недостающие памятки, по одной с паузой.
func (s *Service) BackfillLoop(ctx context.Context) {
	for {
		s.backfill(ctx)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backfillEvery):
		}
	}
}

func (s *Service) backfill(ctx context.Context) {
	list, err := s.pending(ctx)
	if err != nil {
		log.Printf("памятки: не удалось получить список лекарств: %v", err)
		return
	}
	for i, m := range list {
		if i > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backfillPause):
			}
		}
		err := s.Ensure(ctx, m.id)
		if err == nil {
			log.Printf("памятка готова: %s", m.name)
			continue
		}
		log.Printf("памятка не создана: %s: %v", m.name, err)
		// Неверный ключ или нет доступа — остальные лекарства не пробуем до следующего прохода.
		if errors.Is(err, errNoAccess) {
			return
		}
	}
}
