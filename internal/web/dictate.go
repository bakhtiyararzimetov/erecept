package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"erecept/internal/dictate"
)

// Dictator разбирает надиктованный рецепт (dictate.Parser; в тестах — подделка).
type Dictator interface {
	Parse(ctx context.Context, text string, catalog []string) (dictate.Draft, error)
}

// dictate — кнопка «Заполнить с ИИ»: форма приходит целиком, ИИ дополняет её по диктовке.
// Рецепт не сохраняется — врач проверяет поля и нажимает «Сохранить рецепт».
func (w *Web) dictate(rw http.ResponseWriter, r *http.Request) {
	f, _ := parseForm(r)
	f.Dictation = r.PostFormValue("dictation")
	f.BadFields = map[string]bool{}
	for i := range f.Items {
		f.Items[i].Bad = false
	}
	if w.Dictate == nil {
		w.renderForm(rw, r, http.StatusOK, f, []string{tr(r, "Голосовое заполнение не настроено. Заполните рецепт вручную.")})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	d, err := w.Dictate.Parse(ctx, f.Dictation, w.medicineNames(ctx))
	switch {
	case errors.Is(err, dictate.ErrEmpty):
		w.renderForm(rw, r, http.StatusOK, f, []string{tr(r, "Сначала надиктуйте или впишите текст рецепта.")})
		return
	case err != nil:
		log.Printf("диктовка: %v", err)
		w.renderForm(rw, r, http.StatusOK, f, []string{tr(r, "ИИ не смог разобрать диктовку. Попробуйте ещё раз или заполните вручную.")})
		return
	case d.PatientName == "" && len(d.Items) == 0:
		w.renderForm(rw, r, http.StatusOK, f, []string{tr(r, "В диктовке не нашлось ни пациента, ни лекарств. Скажите, например: «Иванова Галина, амлодипин 5 мг утром после еды, 30 дней».")})
		return
	}
	mergeDraft(&f, d)
	w.renderStatus(rw, r, http.StatusOK, "doctor_new", map[string]any{
		"Form": f, "Medicines": w.medicineNames(r.Context()), "CanDictate": true, "Dictated": true,
	})
}

// mergeDraft переносит черновик в форму: сказанное заменяет поля, лекарства добавляются
// вместо пустых строк.
func mergeDraft(f *RxForm, d dictate.Draft) {
	if d.PatientName != "" {
		f.PatientName = d.PatientName
	}
	if d.PatientBirth != "" {
		f.PatientBirth = d.PatientBirth
	}
	if d.Comment != "" {
		if f.Comment != "" {
			f.Comment += "\n"
		}
		f.Comment += d.Comment
	}
	var items []ItemForm
	for _, it := range f.Items {
		if it.Name != "" {
			items = append(items, it)
		}
	}
	next := f.NextIndex()
	for _, di := range d.Items {
		if len(items) >= maxItems {
			break
		}
		it := ItemForm{I: next, Name: di.Name, Dosage: di.Dosage, TimesPerDay: di.TimesPerDay, Meal: di.Meal, Times: di.Times}
		if it.Times == "" {
			it.Times = defaultTimes[it.TimesPerDay]
		}
		if di.Days > 0 {
			it.Days = strconv.Itoa(di.Days)
		}
		items = append(items, it)
		next++
	}
	if len(items) == 0 {
		items = []ItemForm{newItemForm(next)}
	}
	f.Items = items
}
