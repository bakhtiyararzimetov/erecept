package web

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// ErrCSVHeader — файл нельзя разобрать из-за заголовка (нет строки или нет нужных колонок).
var ErrCSVHeader = errors.New("проблема с заголовком CSV")

const (
	maxCSVBytes = 2 << 20
	maxCSVRows  = 5000
	errTooBig   = "Файл слишком большой: не больше 2 МБ и 5000 строк"
)

var errCSVTooBig = errors.New(errTooBig)

// CSVRow — одна валидная строка файла аптеки.
type CSVRow struct {
	Name, Substance, Form string
	Price                 int
	InStock               bool
}

const sampleCSV = "\ufeffназвание;действующее вещество;форма;цена;наличие\r\n" +
	"Парацетамол;парацетамол;таблетки 500 мг;350;да\r\n" +
	"Амоксициллин;амоксициллин;капсулы 500 мг;1250;да\r\n" +
	"Нурофен;ибупрофен;сироп;2100;нет\r\n"

func normHeader(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// parsePrice: «1 250», «1 250 ₸», «350 тг» → целое ≥ 0.
func parsePrice(s string) (int, error) {
	s = strings.ToLower(s)
	s = strings.NewReplacer("₸", "", "тенге", "", "тг.", "", "тг", "").Replace(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	if s == "" || len(s) > 9 {
		return 0, errors.New("не число")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, errors.New("не число")
	}
	return n, nil
}

func parseStock(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "да", "есть", "1", "true":
		return true, true
	case "нет", "0", "false":
		return false, true
	}
	return false, false
}

// ParseCSV читает файл аптеки. Валидные строки возвращаются, по битым — ошибки с номерами строк.
func ParseCSV(r io.Reader) (rows []CSVRow, errs []string) {
	rows, errs, _ = parseCSV(r)
	return rows, errs
}

// parseCSV — как ParseCSV, но фатальную ошибку (ErrCSVHeader, слишком большой файл) отдаёт отдельно.
func parseCSV(r io.Reader) (rows []CSVRow, errs []string, fatal error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, []string{"Не удалось прочитать файл"}, err
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if !utf8.Valid(data) { // русский Excel часто сохраняет в Windows-1251
		if dec, err := charmap.Windows1251.NewDecoder().Bytes(data); err == nil {
			data = dec
		}
	}
	text := strings.TrimLeft(string(data), "\r\n\t ")
	header, _, _ := strings.Cut(text, "\n")
	comma := ','
	if strings.Count(header, ";") >= 1 {
		comma = ';'
	}
	cr := csv.NewReader(strings.NewReader(text))
	cr.Comma = comma
	cr.LazyQuotes = true
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true

	head, err := cr.Read()
	if err != nil {
		return nil, []string{"В файле нет строки-заголовка: название;действующее вещество;форма;цена;наличие"}, ErrCSVHeader
	}
	col := map[string]int{}
	for i, h := range head {
		if _, dup := col[normHeader(h)]; !dup {
			col[normHeader(h)] = i
		}
	}
	var missing []string
	for _, need := range []string{"название", "цена"} {
		if _, ok := col[need]; !ok {
			missing = append(missing, "«"+need+"»")
		}
	}
	if len(missing) > 0 {
		return nil, []string{"В заголовке нет колонки " + strings.Join(missing, ", ") + ". Нужны: название;действующее вещество;форма;цена;наличие"}, ErrCSVHeader
	}
	cell := func(rec []string, name string) string {
		i, ok := col[name]
		if !ok || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	n := 0
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		line, _ := cr.FieldPos(0)
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				line = pe.StartLine
			}
			errs = append(errs, fmt.Sprintf("строка %d: не удалось разобрать строку", line))
			continue
		}
		if n++; n > maxCSVRows {
			return nil, []string{errTooBig}, errCSVTooBig
		}
		empty := true
		for _, c := range rec {
			if strings.TrimSpace(c) != "" {
				empty = false
			}
		}
		if empty {
			continue
		}
		if strings.ContainsRune(strings.Join(rec, ""), 0) {
			errs = append(errs, fmt.Sprintf("строка %d: недопустимые символы", line))
			continue
		}
		row := CSVRow{Name: cell(rec, "название"), Substance: cell(rec, "действующее вещество"), Form: cell(rec, "форма")}
		if row.Name == "" {
			errs = append(errs, fmt.Sprintf("строка %d: пустое название", line))
			continue
		}
		if len([]rune(row.Name)) > 200 {
			errs = append(errs, fmt.Sprintf("строка %d: название длиннее 200 символов", line))
			continue
		}
		raw := cell(rec, "цена")
		if row.Price, err = parsePrice(raw); err != nil {
			errs = append(errs, fmt.Sprintf("строка %d: цена «%s» — не число", line, raw))
			continue
		}
		stock := cell(rec, "наличие")
		var ok bool
		if row.InStock, ok = parseStock(stock); !ok {
			errs = append(errs, fmt.Sprintf("строка %d: наличие «%s» — ожидается да или нет", line, stock))
			continue
		}
		rows = append(rows, row)
	}
	return rows, errs, nil
}
