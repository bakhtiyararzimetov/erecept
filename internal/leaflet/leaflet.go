// Package leaflet: клиент реестра НЦЭЛС — поиск лекарства по торговому названию
// и получение текста официальной инструкции (DOCX) в виде простого текста.
package leaflet

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://register.ndda.kz/register/api/v1/PharmaRegister"
	siteURL        = "https://register.ndda.kz/"
	maxBody        = 20 << 20 // больше 20 МБ ответ не читаем
)

// ErrNotFound: в реестре нет лекарства с таким названием.
var ErrNotFound = errors.New("лекарство не найдено в реестре")

// ErrWrongMNN: по названию что-то нашлось, но ни у одной записи нет нужного МНН.
var ErrWrongMNN = errors.New("в реестре нет препарата с таким МНН")

// ErrWrongForm: нет записи в той же лекарственной форме (или форма в справочнике не указана).
var ErrWrongForm = errors.New("в реестре нет препарата в такой форме")

// ErrNoInstruction: у записи нет файла инструкции, или это не DOCX, или в нём нет текста.
var ErrNoInstruction = errors.New("у записи нет инструкции")

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func New() *Client {
	return &Client{BaseURL: defaultBaseURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

type Entry struct {
	RegisterID int64
	TradeName  string
	RegNumber  string
	MNN        string
	Form       string     // лекарственная форма: «Таблетки, покрытые оболочкой»
	ExpiresAt  *time.Time // срок регистрации; nil — бессрочно или не указан
	AtcCode    string     // код АТХ, например «M01AB05»
}

type listItem struct {
	ID             int64   `json:"id"`
	DrugTradeName  string  `json:"drugTradeName"`
	RegNumber      string  `json:"regNumber"`
	MNN            string  `json:"mnn"`
	LekFormName    string  `json:"lekFormName"`
	ExpirationDate *string `json:"expirationDate"` // «2028-07-11T00:00:00» или null
	AtcCode        string  `json:"atcCode"`
}

// norm приводит название или МНН к виду для сравнения: без «®», в нижнем регистре,
// пробелы по краям убраны, внутри схлопнуты до одного.
func norm(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "®", ""))), " ")
}

func (it listItem) entry() Entry {
	e := Entry{RegisterID: it.ID, TradeName: strings.TrimSpace(it.DrugTradeName), RegNumber: it.RegNumber, MNN: it.MNN,
		Form: strings.TrimSpace(it.LekFormName), AtcCode: strings.TrimSpace(it.AtcCode)}
	if it.ExpirationDate != nil {
		// Неразборчивая дата считается как «не указан».
		if t, err := time.ParseInLocation("2006-01-02T15:04:05", *it.ExpirationDate, time.UTC); err == nil {
			e.ExpiresAt = &t
		}
	}
	return e
}

// formRoot — корень формы для сравнения: первые 6 букв первого слова в нижнем регистре
// («таблетки 500 мг» → «таблет», «Капсулы кишечнорастворимые» → «капсул»).
func formRoot(form string) string {
	f := strings.Fields(strings.ToLower(form))
	if len(f) == 0 {
		return ""
	}
	r := []rune(strings.TrimRight(f[0], ",.;:"))
	if len(r) > 6 {
		r = r[:6]
	}
	return string(r)
}

// Find ищет запись по торговому названию: точное совпадение, иначе первая в ответе.
func (c *Client) Find(ctx context.Context, tradeName string) (Entry, error) {
	items, err := c.list(ctx, tradeName)
	if err != nil {
		return Entry{}, err
	}
	pick := items[0]
	want := norm(tradeName)
	for _, it := range items {
		if norm(it.DrugTradeName) == want {
			pick = it
			break
		}
	}
	return pick.entry(), nil
}

// FindFor — лучшая запись из CandidatesFor.
func (c *Client) FindFor(ctx context.Context, tradeName, substance, form string) (Entry, error) {
	list, err := c.CandidatesFor(ctx, tradeName, substance, form)
	if err != nil {
		return Entry{}, err
	}
	return list[0], nil
}

// CandidatesFor — записи реестра с тем же МНН (substance) и той же лекарственной формой (form
// из справочника, сравнивается корень первого слова), по порядку предпочтения: сначала действующие
// регистрации (без срока или срок в будущем) — точное торговое название, затем порядок реестра;
// потом истёкшие — от самого позднего срока (у старых регистраций файла инструкции обычно нет).
// Пустое МНН — ErrWrongMNN, пустая форма — ErrWrongForm, без запроса в реестр: угадывать нельзя.
// Список нужен, чтобы взять следующую запись, если у первой нет файла инструкции.
func (c *Client) CandidatesFor(ctx context.Context, tradeName, substance, form string) ([]Entry, error) {
	mnn, root := norm(substance), formRoot(form)
	if mnn == "" {
		return nil, ErrWrongMNN
	}
	if root == "" {
		return nil, ErrWrongForm
	}
	items, err := c.list(ctx, tradeName)
	if err != nil {
		return nil, err
	}
	want := norm(tradeName)
	now := time.Now()
	type ranked struct {
		e       Entry
		expired bool
		exact   bool
	}
	var list []ranked
	sameMNN := false
	for _, it := range items {
		if norm(it.MNN) != mnn {
			continue
		}
		sameMNN = true
		if formRoot(it.LekFormName) != root {
			continue
		}
		e := it.entry()
		list = append(list, ranked{e: e, expired: e.ExpiresAt != nil && e.ExpiresAt.Before(now), exact: norm(it.DrugTradeName) == want})
	}
	if !sameMNN {
		return nil, ErrWrongMNN
	}
	if len(list) == 0 {
		return nil, ErrWrongForm
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.expired != b.expired {
			return !a.expired
		}
		if a.expired && !a.e.ExpiresAt.Equal(*b.e.ExpiresAt) {
			return a.e.ExpiresAt.After(*b.e.ExpiresAt)
		}
		return a.exact && !b.exact
	})
	out := make([]Entry, len(list))
	for i, r := range list {
		out[i] = r.e
	}
	return out, nil
}

// list — записи реестра по торговому названию; пустой ответ — ErrNotFound.
func (c *Client) list(ctx context.Context, tradeName string) ([]listItem, error) {
	body, err := json.Marshal(map[string]string{"tradeName": tradeName})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/list", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	data, err := c.do(req)
	if err != nil {
		return nil, err
	}
	var items []listItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("реестр вернул не список: %w", err)
	}
	if len(items) == 0 {
		return nil, ErrNotFound
	}
	return items, nil
}

// Instruction скачивает инструкцию (DOCX) и возвращает её текст, абзацы через перевод строки.
func (c *Client) Instruction(ctx context.Context, registerID int64) (string, error) {
	u := c.BaseURL + "/getRegisterInstructionFile?registerId=" + strconv.FormatInt(registerID, 10) + "&lang=" + url.QueryEscape("ru")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	data, err := c.do(req)
	if err != nil {
		return "", err
	}
	text, err := docxText(data)
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("%w: в DOCX нет текста", ErrNoInstruction)
	}
	return text, nil
}

// SourceURL: ссылка для людей. Стабильного пути к записи в реестре нет (это SPA),
// поэтому ведём на главную страницу реестра.
func SourceURL(e Entry) string {
	return siteURL
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("реестр недоступен: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("реестр ответил статусом %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать ответ реестра: %w", err)
	}
	return data, nil
}

// docxText достаёт текст из word/document.xml: <w:t> — текст, конец <w:p> — перевод строки.
func docxText(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("%w: ответ реестра не DOCX", ErrNoInstruction)
	}
	var doc *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			doc = f
			break
		}
	}
	if doc == nil {
		return "", fmt.Errorf("%w: в DOCX нет word/document.xml", ErrNoInstruction)
	}
	rc, err := doc.Open()
	if err != nil {
		return "", fmt.Errorf("не удалось открыть DOCX: %w", err)
	}
	defer rc.Close()

	dec := xml.NewDecoder(io.LimitReader(rc, maxBody))
	var sb strings.Builder
	inText := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w: битый XML в DOCX: %v", ErrNoInstruction, err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "t":
				inText = true
			case "tab":
				sb.WriteByte(' ')
			case "br":
				sb.WriteByte('\n')
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				inText = false
			case "p":
				sb.WriteByte('\n')
			}
		case xml.CharData:
			if inText {
				sb.Write(t)
			}
		}
	}
	var lines []string
	for _, l := range strings.Split(sb.String(), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n"), nil
}
