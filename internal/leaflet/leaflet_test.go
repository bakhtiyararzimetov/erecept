package leaflet

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(h http.HandlerFunc) (*Client, func()) {
	srv := httptest.NewServer(h)
	return &Client{BaseURL: srv.URL, HTTP: srv.Client()}, srv.Close
}

func TestFindExactMatch(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/list" {
			t.Errorf("неожиданный запрос: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"id":1,"drugTradeName":"Нурофен® Форте для детей","regNumber":"A","mnn":"ибупрофен"},
			{"id":2,"drugTradeName":" Нурофен® ","regNumber":"РК-ЛС-5№1","mnn":"ибупрофен"}]`))
	})
	defer stop()
	e, err := c.Find(context.Background(), "нурофен")
	if err != nil {
		t.Fatal(err)
	}
	if e.RegisterID != 2 || e.RegNumber != "РК-ЛС-5№1" || e.MNN != "ибупрофен" {
		t.Fatalf("выбрана не та запись: %+v", e)
	}
}

func TestFindFirstWhenNoExact(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":7,"drugTradeName":"Нурофен® Экспресс"},{"id":8,"drugTradeName":"Нурофен® Плюс"}]`))
	})
	defer stop()
	e, err := c.Find(context.Background(), "Нурофен")
	if err != nil || e.RegisterID != 7 {
		t.Fatalf("ожидалась первая запись: %+v, %v", e, err)
	}
}

func TestFindNotFound(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[]`))
	})
	defer stop()
	if _, err := c.Find(context.Background(), "Нет такого"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидался ErrNotFound, получено %v", err)
	}
}

func makeDocx(t *testing.T, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="x"><w:body>` + body + `</w:body></w:document>`))
	zw.Close()
	return buf.Bytes()
}

func TestInstructionDocx(t *testing.T) {
	docx := makeDocx(t, `<w:p><w:r><w:t>Показания</w:t></w:r></w:p><w:p></w:p>`+
		`<w:p><w:r><w:t>Способ </w:t></w:r><w:r><w:t>применения &amp; дозы</w:t></w:r></w:p>`)
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getRegisterInstructionFile" || r.URL.Query().Get("registerId") != "42" || r.URL.Query().Get("lang") != "ru" {
			t.Errorf("неожиданный запрос: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		w.Write(docx)
	})
	defer stop()
	text, err := c.Instruction(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Способ применения & дозы") {
		t.Fatalf("нет текста: %q", text)
	}
	if text != "Показания\nСпособ применения & дозы" {
		t.Fatalf("неверное разбиение на абзацы: %q", text)
	}
}

func TestInstructionNotDocx(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html>Ошибка</html>`))
	})
	defer stop()
	if _, err := c.Instruction(context.Background(), 1); !errors.Is(err, ErrNoInstruction) {
		t.Fatalf("ожидался ErrNoInstruction, получено %v", err)
	}
}

func TestInstructionEmpty(t *testing.T) {
	docx := makeDocx(t, `<w:p></w:p>`)
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) { w.Write(docx) })
	defer stop()
	if _, err := c.Instruction(context.Background(), 1); !errors.Is(err, ErrNoInstruction) {
		t.Fatalf("ожидался ErrNoInstruction для пустого текста, получено %v", err)
	}
}

func TestSourceURL(t *testing.T) {
	if got := SourceURL(Entry{RegisterID: 5}); got != "https://register.ndda.kz/" {
		t.Fatal(got)
	}
}

const tablets = "таблетки 500 мг №10"

func TestFindFor(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"id":1,"drugTradeName":"Парацетамол-Плюс","regNumber":"A","mnn":"Парацетамол, Кофеин","lekFormName":"Таблетки"},
			{"id":2,"drugTradeName":"Парацетамол","regNumber":"B","mnn":"Парацетамол, Аскорбиновая кислота","lekFormName":"Таблетки"},
			{"id":3,"drugTradeName":"Парацетамол Детский","regNumber":"C","mnn":" Парацетамол ","lekFormName":"Таблетки"},
			{"id":4,"drugTradeName":"Парацетамол®","regNumber":"D","mnn":"парацетамол","lekFormName":"Таблетки","atcCode":"N02BE01"}]`))
	})
	defer stop()
	ctx := context.Background()

	// Точное название + совпадающее МНН важнее первого с тем же МНН.
	e, err := c.FindFor(ctx, "парацетамол", "Парацетамол", tablets)
	if err != nil || e.RegisterID != 4 || e.AtcCode != "N02BE01" || e.Form != "Таблетки" {
		t.Fatalf("ожидалась запись 4: %+v, %v", e, err)
	}
	// Точного названия с этим МНН нет — первая запись с совпадающим МНН.
	e, err = c.FindFor(ctx, "Панадол", "парацетамол", tablets)
	if err != nil || e.RegisterID != 3 {
		t.Fatalf("ожидалась запись 3: %+v, %v", e, err)
	}
	// Нет записи с таким МНН — ошибка, а не чужой препарат.
	if _, err := c.FindFor(ctx, "Парацетамол", "варфарин", tablets); !errors.Is(err, ErrWrongMNN) {
		t.Fatalf("ожидался ErrWrongMNN, получено %v", err)
	}
	// Нет записи в такой форме.
	if _, err := c.FindFor(ctx, "Парацетамол", "парацетамол", "сироп 120 мг/5 мл"); !errors.Is(err, ErrWrongForm) {
		t.Fatalf("ожидался ErrWrongForm, получено %v", err)
	}
}

func TestFindForNotFound(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`[]`)) })
	defer stop()
	if _, err := c.FindFor(context.Background(), "Нет", "нет", tablets); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ожидался ErrNotFound, получено %v", err)
	}
}

// Без МНН или формы в реестр не ходим: угадывать препарат нельзя.
func TestCandidatesForNeedsSubstanceAndForm(t *testing.T) {
	calls := 0
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte(`[]`)) })
	defer stop()
	ctx := context.Background()
	if _, err := c.CandidatesFor(ctx, "Парацетамол", "", tablets); !errors.Is(err, ErrWrongMNN) {
		t.Fatalf("пустое МНН: %v", err)
	}
	if _, err := c.CandidatesFor(ctx, "Парацетамол", "парацетамол", " "); !errors.Is(err, ErrWrongForm) {
		t.Fatalf("пустая форма: %v", err)
	}
	if calls != 0 {
		t.Fatalf("реестр вызван %d раз", calls)
	}
}

// Настоящий порядок ответа реестра для «Диклофенак»: свечи и пластырь раньше таблеток.
func TestCandidatesForDiclofenacForm(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"id":99001817,"drugTradeName":"Диклофенак","mnn":"диклофенак натрия","lekFormName":"","expirationDate":null},
			{"id":99001794,"drugTradeName":"Диклофенак-Тева","mnn":"диклофенак","lekFormName":"пластырь","expirationDate":null},
			{"id":182748,"drugTradeName":"Диклофенак","mnn":"Диклофенак","lekFormName":"Суппозитории ректальные","expirationDate":null},
			{"id":179657,"drugTradeName":"Диклофенак-Тева","mnn":"Диклофенак","lekFormName":"Капсулы кишечнорастворимые","expirationDate":"2026-01-29T00:00:00"},
			{"id":180423,"drugTradeName":"Диклофенак","mnn":"Диклофенак","lekFormName":"Раствор для внутримышечного введения","expirationDate":"2028-07-11T00:00:00"},
			{"id":99000180,"drugTradeName":"Диклофенак-Тева","mnn":"диклофенак","lekFormName":"таблетки, покрытые кишечнорастворимой оболочкой","expirationDate":"2029-03-15T00:00:00"}]`))
	})
	defer stop()
	list, err := c.CandidatesFor(context.Background(), "Диклофенак", "диклофенак", "таблетки 50 мг №20")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].RegisterID != 99000180 {
		t.Fatalf("ожидались только таблетки 99000180: %+v", list)
	}
}

// Сначала действующие регистрации (без срока или со сроком в будущем), затем точное название.
func TestCandidatesForPrefersValidRegistration(t *testing.T) {
	c, stop := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[
			{"id":1,"drugTradeName":"Парацетамол","mnn":"парацетамол","lekFormName":"Таблетки","expirationDate":"2020-05-01T00:00:00"},
			{"id":2,"drugTradeName":"Парацетамол Детский","mnn":"парацетамол","lekFormName":"Таблетки","expirationDate":"2099-01-01T00:00:00"},
			{"id":3,"drugTradeName":"ПАРАЦЕТАМОЛ","mnn":"Парацетамол","lekFormName":"Таблетки","expirationDate":null},
			{"id":4,"drugTradeName":"Парацетамол-Д","mnn":"парацетамол","lekFormName":"Таблетки","expirationDate":"2099-01-01T00:00:00"},
			{"id":5,"drugTradeName":"Парацетамол-Е","mnn":"парацетамол","lekFormName":"Таблетки","expirationDate":"2025-12-31T00:00:00"}]`))
	})
	defer stop()
	list, err := c.CandidatesFor(context.Background(), "Парацетамол", "парацетамол", tablets)
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, e := range list {
		ids = append(ids, e.RegisterID)
	}
	// 3 — действующая и точное название; 2, 4 — действующие; истёкшие в конце,
	// сначала с более поздним сроком (у старых регистраций файла инструкции обычно нет).
	if len(ids) != 5 || ids[0] != 3 || ids[1] != 2 || ids[2] != 4 || ids[3] != 5 || ids[4] != 1 {
		t.Fatalf("порядок кандидатов: %v", ids)
	}
	if list[4].ExpiresAt == nil || list[4].ExpiresAt.Year() != 2020 || list[0].ExpiresAt != nil {
		t.Fatalf("срок регистрации не разобран: %+v", list)
	}
}

func TestFormRoot(t *testing.T) {
	for in, want := range map[string]string{
		"таблетки 500 мг №10": "таблет", "Таблетки, покрытые оболочкой": "таблет",
		"капсулы 20 мг": "капсул", "сироп 30 мг/5 мл": "сироп", "Суспензия для приема внутрь": "суспен", "": "",
	} {
		if got := formRoot(in); got != want {
			t.Errorf("formRoot(%q) = %q, ожидалось %q", in, got, want)
		}
	}
}
