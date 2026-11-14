package i18n

import (
	"regexp"
	"strings"
)

// Данные справочника и демо-аптек по-английски: форма выпуска, название и адрес аптеки.

var (
	countRe  = regexp.MustCompile(`\s*№\s*(\d+)`)
	formRepl = strings.NewReplacer("шипучие таблетки", "effervescent tablets", "таблетки", "tablets", "капсулы", "capsules", "сироп", "syrup",
		" мг", " mg", " мл", " ml", " ЕД", " U", "мл ", "ml ")
	placeRepl = strings.NewReplacer("Аптека ", "Pharmacy ", "Шымкент", "Shymkent", "Алматы", "Almaty", "мкр.", "mcr.", "ул.", "st.", "пр.", "ave.")
)

// FormEN: «таблетки 850 мг №60» → «tablets 850 mg, 60 pcs».
func FormEN(s string) string {
	s = formRepl.Replace(s)
	s = strings.ReplaceAll(s, "/5 мл", "/5 ml")
	return countRe.ReplaceAllString(s, ", $1 pcs")
}

// PlaceEN — название или адрес аптеки по-английски (имена собственные остаются).
func PlaceEN(s string) string { return placeRepl.Replace(s) }

// HoursEN — часы работы аптеки по-английски.
func HoursEN(s string) string {
	if s == "Круглосуточно" {
		return "24 hours"
	}
	return s
}

// Form, Place и Hours — то же для языка l (по-русски строка не меняется).
func (l Lang) Form(s string) string {
	if l == EN {
		return FormEN(s)
	}
	return s
}

func (l Lang) Place(s string) string {
	if l == EN {
		return PlaceEN(s)
	}
	return s
}

func (l Lang) Hours(s string) string {
	if l == EN {
		return HoursEN(s)
	}
	return s
}

// Pick — английский вариант en на английском сайте, если он есть; иначе ru.
func (l Lang) Pick(ru, en string) string {
	if l == EN && en != "" {
		return en
	}
	return ru
}
