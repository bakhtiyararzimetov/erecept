package web

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"erecept/internal/db/dbtest"
	"erecept/internal/dictate"
	"erecept/internal/seed"
)

type fakeDictator struct {
	draft   dictate.Draft
	err     error
	gotText string
	catalog []string
}

func (f *fakeDictator) Parse(_ context.Context, text string, catalog []string) (dictate.Draft, error) {
	f.gotText, f.catalog = text, catalog
	return f.draft, f.err
}

func dictateEnv(t *testing.T, d Dictator) *Web {
	t.Helper()
	pool := dbtest.New(t)
	if err := seed.Run(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	w := New(pool)
	if d != nil {
		w.Dictate = d
	}
	return w
}

// Диктовка заполняет форму, но рецепт не сохраняет: врач проверяет и сохраняет сам.
func TestDictateFillsFormWithoutSaving(t *testing.T) {
	fake := &fakeDictator{draft: dictate.Draft{
		PatientName: "Иванова Галина Петровна", PatientBirth: "14.03.1956", Comment: "Контроль давления через месяц",
		Items: []dictate.Item{
			{Name: "Амлодипин", Dosage: "1 таблетка 5 мг", TimesPerDay: 1, Times: "08:00", Meal: "after", Days: 30},
			{Name: "Бисопролол", Dosage: "1 таблетка 5 мг", TimesPerDay: 2, Meal: "before"},
		},
	}}
	w := dictateEnv(t, fake)
	h := w.Routes()
	cookies := doctorCookies(t, h, "doctor1")
	var before int
	w.DB.QueryRow(context.Background(), `SELECT count(*) FROM prescriptions`).Scan(&before)

	form := url.Values{
		"dictation":              {"Иванова Галина Петровна, амлодипин пять миллиграмм утром"},
		"patient_name":           {""},
		"items[0].name":          {""},
		"items[0].times_per_day": {"1"},
		"items[0].days":          {"7"},
	}
	resp := postForm(h, "/doctor/dictate", form, cookies)
	body := resp.Body.String()
	if resp.Code != 200 {
		t.Fatalf("код %d", resp.Code)
	}
	for _, want := range []string{`value="Иванова Галина Петровна"`, `value="14.03.1956"`, "Контроль давления через месяц",
		`name="items[1].name" list="medicines" value="Амлодипин"`, `name="items[2].name" list="medicines" value="Бисопролол"`,
		`name="items[1].days" type="number" min="1" max="365" value="30"`,
		`name="items[2].times" class="times" value="08:00, 20:00"`, // время по умолчанию для «2 раза в день»
		"Проверьте каждое поле", "Иванова Галина Петровна, амлодипин пять миллиграмм утром"} {
		if !strings.Contains(body, want) {
			t.Errorf("на странице нет %q", want)
		}
	}
	if strings.Contains(body, `name="items[0].name"`) {
		t.Error("пустая строка лекарства должна замениться надиктованными")
	}
	if fake.gotText != "Иванова Галина Петровна, амлодипин пять миллиграмм утром" || len(fake.catalog) == 0 {
		t.Errorf("в ИИ ушло %q, справочник %d", fake.gotText, len(fake.catalog))
	}
	var after int
	w.DB.QueryRow(context.Background(), `SELECT count(*) FROM prescriptions`).Scan(&after)
	if after != before {
		t.Error("диктовка не должна сохранять рецепт")
	}
}

func TestDictateErrorsKeepForm(t *testing.T) {
	w := dictateEnv(t, &fakeDictator{err: errors.New("сбой")})
	h := w.Routes()
	cookies := doctorCookies(t, h, "doctor1")
	form := validForm()
	form.Set("dictation", "что-то")
	body := postForm(h, "/doctor/dictate", form, cookies).Body.String()
	if !strings.Contains(body, "ИИ не смог разобрать диктовку") || !strings.Contains(body, `value="Иванов Иван Иванович"`) {
		t.Error("при ошибке ИИ форма должна сохраниться и показать ошибку")
	}

	// Без ключа ИИ кнопки нет, а запрос отвечает понятной ошибкой.
	w = dictateEnv(t, nil)
	h = w.Routes()
	cookies = doctorCookies(t, h, "doctor1")
	if strings.Contains(get(h, "/doctor/", cookies).Body.String(), "Заполнить с ИИ") {
		t.Error("без ключа кнопки диктовки быть не должно")
	}
	if !strings.Contains(postForm(h, "/doctor/dictate", form, cookies).Body.String(), "Голосовое заполнение не настроено") {
		t.Error("нет сообщения о выключенной диктовке")
	}

	// Аптеке и гостю диктовка недоступна.
	if resp := postForm(h, "/doctor/dictate", form, nil); resp.Code == 200 {
		t.Error("гость не должен попадать в диктовку")
	}
}
