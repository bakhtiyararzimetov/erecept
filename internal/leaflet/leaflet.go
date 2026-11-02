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
}

type listItem struct {
	ID            int64  `json:"id"`
	DrugTradeName string `json:"drugTradeName"`
	RegNumber     string `json:"regNumber"`
	MNN           string `json:"mnn"`
}

// norm приводит название к виду для сравнения: без «®», пробелов по краям, в нижнем регистре.
func norm(s string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "®", "")))
}

// Find ищет запись по торговому названию: точное совпадение, иначе первая в ответе.
func (c *Client) Find(ctx context.Context, tradeName string) (Entry, error) {
	body, err := json.Marshal(map[string]string{"tradeName": tradeName})
	if err != nil {
		return Entry{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/list", bytes.NewReader(body))
	if err != nil {
		return Entry{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	data, err := c.do(req)
	if err != nil {
		return Entry{}, err
	}
	var items []listItem
	if err := json.Unmarshal(data, &items); err != nil {
		return Entry{}, fmt.Errorf("реестр вернул не список: %w", err)
	}
	if len(items) == 0 {
		return Entry{}, ErrNotFound
	}
	pick := items[0]
	want := norm(tradeName)
	for _, it := range items {
		if norm(it.DrugTradeName) == want {
			pick = it
			break
		}
	}
	return Entry{RegisterID: pick.ID, TradeName: strings.TrimSpace(pick.DrugTradeName), RegNumber: pick.RegNumber, MNN: pick.MNN}, nil
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
		return "", errors.New("в инструкции нет текста")
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
		return "", errors.New("ответ реестра не DOCX")
	}
	var doc *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			doc = f
			break
		}
	}
	if doc == nil {
		return "", errors.New("в DOCX нет word/document.xml")
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
			return "", fmt.Errorf("битый XML в DOCX: %w", err)
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
