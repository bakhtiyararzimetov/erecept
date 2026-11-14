package web

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"

	"erecept/internal/i18n"
)

func TestParseCSV(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		want     []CSVRow
		errParts []string // подстроки ожидаемых ошибок, по порядку
	}{
		{"точка с запятой и BOM", "\ufeffназвание;действующее вещество;форма;цена;наличие\nПарацетамол;парацетамол;таблетки;350;да\n",
			[]CSVRow{{"Парацетамол", "парацетамол", "таблетки", 350, true}}, nil},
		{"запятая, порядок и регистр колонок", "  Цена , НАЗВАНИЕ \n 500 , Аспирин \n",
			[]CSVRow{{"Аспирин", "", "", 500, true}}, nil},
		{"цена с пробелами и ₸", "название;цена\nИбупрофен;1 250 ₸\nНо-шпа;2 100 тг\n",
			[]CSVRow{{"Ибупрофен", "", "", 1250, true}, {"Но-шпа", "", "", 2100, true}}, nil},
		{"наличие нет/0/false", "название;цена;наличие\nА;1;нет\nБ;2;0\nВ;3;false\nГ;4;\nД;5;есть\n",
			[]CSVRow{{"А", "", "", 1, false}, {"Б", "", "", 2, false}, {"В", "", "", 3, false}, {"Г", "", "", 4, true}, {"Д", "", "", 5, true}}, nil},
		{"битые строки", "название;цена\nА;10\nБ;abc\n;5\nВ;-3\nГ;7\n\n;;\n",
			[]CSVRow{{"А", "", "", 10, true}, {"Г", "", "", 7, true}},
			[]string{"строка 3: цена «abc» — не число", "строка 4: пустое название", "строка 5"}},
		{"нет колонки цена", "название;форма\nА;таблетки\n", nil, []string{"цена"}},
		{"пустой файл", "", nil, []string{"заголовк"}},
		{"кавычки и разделитель внутри", "название;цена\n\"Лекарство; особое\";100\n",
			[]CSVRow{{"Лекарство; особое", "", "", 100, true}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, errs := ParseCSV(strings.NewReader(c.in))
			if len(rows) != len(c.want) {
				t.Fatalf("строк %d, ждали %d: %+v", len(rows), len(c.want), rows)
			}
			for i := range rows {
				if rows[i] != c.want[i] {
					t.Errorf("строка %d: %+v, ждали %+v", i, rows[i], c.want[i])
				}
			}
			if len(errs) != len(c.errParts) {
				t.Fatalf("ошибок %d, ждали %d: %v", len(errs), len(c.errParts), errs)
			}
			for i, p := range c.errParts {
				if !strings.Contains(errs[i], p) {
					t.Errorf("ошибка %d: %q не содержит %q", i, errs[i], p)
				}
			}
		})
	}
}

func TestParseCSVTooManyRows(t *testing.T) {
	var b strings.Builder
	b.WriteString("название;цена\n")
	for i := 0; i < maxCSVRows+1; i++ {
		b.WriteString("Х;1\n")
	}
	rows, errs := ParseCSV(strings.NewReader(b.String()))
	if len(rows) != 0 || len(errs) != 1 || !strings.Contains(errs[0], "Файл слишком большой") {
		t.Fatalf("rows=%d errs=%v", len(rows), errs)
	}
}

func TestParseCSVWindows1251AndOddInput(t *testing.T) {
	enc, err := charmap.Windows1251.NewEncoder().String("название;цена\nПарацетамол;350\n")
	if err != nil {
		t.Fatal(err)
	}
	rows, errs := ParseCSV(strings.NewReader(enc))
	if len(errs) != 0 || len(rows) != 1 || rows[0].Name != "Парацетамол" || rows[0].Price != 350 {
		t.Fatalf("cp1251: rows=%+v errs=%v", rows, errs)
	}
	rows, errs = ParseCSV(strings.NewReader("\n\r\n  \nназвание;цена\nА;1\n"))
	if len(rows) != 1 || len(errs) != 0 {
		t.Fatalf("пустые строки в начале: %+v %v", rows, errs)
	}
	rows, errs = ParseCSV(strings.NewReader("\n\r\n  \nназвание;цена\nА;1\nБ;abc\n"))
	if len(rows) != 1 || len(errs) != 1 || !strings.HasPrefix(errs[0], "строка 6:") {
		t.Fatalf("номер строки после пустых: %+v %v", rows, errs)
	}
	rows, errs = ParseCSV(strings.NewReader("название;цена\nА\x00;1\nБ;2\n"))
	if len(rows) != 1 || len(errs) != 1 || !strings.Contains(errs[0], "строка 2: недопустимые символы") {
		t.Fatalf("NUL: %+v %v", rows, errs)
	}
	if _, _, fatal := parseCSV(strings.NewReader("название;форма\nА;б\n"), i18n.RU); !errors.Is(fatal, ErrCSVHeader) {
		t.Fatalf("ждали ErrCSVHeader, получили %v", fatal)
	}
}
