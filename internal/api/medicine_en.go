package api

import (
	"regexp"
	"strings"

	"erecept/internal/i18n"
)

// Английская версия справочника для приложения и картинки упаковок.

const analogNoteEN = "Agree on a replacement with your doctor or pharmacist"

// medCols — колонки справочника; scanMed дополняет MedicineDTO по языку.
const medCols = `id, name, active_substance, form, name_en, substance_en`

type medRow struct {
	MedicineDTO
	nameEN, substanceEN string
}

func (m *medRow) dest() []any {
	return []any{&m.ID, &m.Name, &m.ActiveSubstance, &m.Form, &m.nameEN, &m.substanceEN}
}

// finish — картинка упаковки и английские поля для английского приложения.
func (m *medRow) finish(l i18n.Lang) MedicineDTO {
	d := m.MedicineDTO
	d.Image = packImage(l, m.nameEN)
	if l == i18n.EN {
		if m.nameEN != "" {
			d.Name = m.nameEN
		}
		if m.substanceEN != "" {
			d.ActiveSubstance = m.substanceEN
		}
		d.Form = formEN(d.Form)
	}
	return d
}

// packImage — картинка упаковки; в английском приложении — вариант без кириллицы (meds/en).
func packImage(l i18n.Lang, nameEN string) string {
	if nameEN == "" {
		return ""
	}
	if l == i18n.EN {
		return "/static/meds/en/" + slug(nameEN) + ".png"
	}
	return "/static/meds/" + slug(nameEN) + ".png"
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slug: «Flemoxin Solutab» → «flemoxin-solutab» (имя файла картинки).
func slug(s string) string {
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

var (
	countRe  = regexp.MustCompile(`\s*№\s*(\d+)`)
	formRepl = strings.NewReplacer("шипучие таблетки", "effervescent tablets", "таблетки", "tablets", "капсулы", "capsules", "сироп", "syrup",
		" мг", " mg", " мл", " ml", " ЕД", " U", "мл ", "ml ")
)

// formEN: «таблетки 850 мг №60» → «tablets 850 mg, 60 pcs».
func formEN(s string) string {
	s = formRepl.Replace(s)
	s = strings.ReplaceAll(s, "/5 мл", "/5 ml")
	return countRe.ReplaceAllString(s, ", $1 pcs")
}

var placeRepl = strings.NewReplacer("Аптека ", "Pharmacy ", "Шымкент", "Shymkent", "Алматы", "Almaty", "мкр.", "mcr.", "ул.", "st.", "пр.", "ave.")

// pharmacyEN — название и адрес тестовой аптеки по-английски (имена собственные остаются).
func pharmacyEN(p *PharmacyDTO) {
	p.Name = placeRepl.Replace(p.Name)
	p.Address = placeRepl.Replace(p.Address)
	if p.WorkHours == "Круглосуточно" {
		p.WorkHours = "24 hours"
	}
}
