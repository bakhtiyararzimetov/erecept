package i18n

import (
	"regexp"
	"strings"
	"unicode"
)

// Имена людей и клиник на английском сайте и в английском приложении: латиница вместо кириллицы.

var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z",
	'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r",
	'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
	// казахские буквы
	'ә': "a", 'ғ': "g", 'қ': "q", 'ң': "ng", 'ө': "o", 'ұ': "u", 'ү': "u", 'һ': "h", 'і': "i",
}

// Translit переводит кириллицу в латиницу; прочие символы не трогает.
func Translit(s string) string {
	var b strings.Builder
	for _, r := range s {
		lower := unicode.ToLower(r)
		lat, ok := translit[lower]
		if !ok {
			b.WriteRune(r)
			continue
		}
		if r != lower && lat != "" { // заглавная: «Ж» → «Zh»
			lat = strings.ToUpper(lat[:1]) + lat[1:]
		}
		b.WriteString(lat)
	}
	return b.String()
}

// Person — имя человека для языка l: на английском латиницей.
func Person(l Lang, s string) string {
	if l == EN {
		return Translit(s)
	}
	return s
}

var clinicWords = []struct{ ru, en string }{
	{"Городская поликлиника", "City Polyclinic"},
	{"городская поликлиника", "City Polyclinic"},
	{"Поликлиника", "Polyclinic"},
	{"поликлиника", "polyclinic"},
	{"Больница", "Hospital"},
	{"больница", "hospital"},
	{"№ ", "No. "},
	{"№", "No. "},
}

// Clinic — название клиники для языка l: частые слова переводятся, остальное латиницей.
func Clinic(l Lang, s string) string {
	if l != EN {
		return s
	}
	for _, w := range clinicWords {
		s = strings.ReplaceAll(s, w.ru, w.en)
	}
	return Translit(s)
}

var dosageWords = []struct {
	re *regexp.Regexp
	en string
}{
	{regexp.MustCompile(`^(½|1/2|0[.,]5)\s+таблетк[аиу]`), "$1 tablet"},
	{regexp.MustCompile(`\b1\s+таблетк[аиу]`), "1 tablet"},
	{regexp.MustCompile(`таблет(ок|ки|ка|ку)`), "tablets"},
	{regexp.MustCompile(`\b1\s+капсул[аыу]`), "1 capsule"},
	{regexp.MustCompile(`капсул(а|ы|у)?`), "capsules"},
	{regexp.MustCompile(`кап(ля|ли|ель|лю)`), "drops"},
	{regexp.MustCompile(`сиропа`), "of syrup"},
	{regexp.MustCompile(`сироп`), "syrup"},
	{regexp.MustCompile(`мг`), "mg"},
	{regexp.MustCompile(`мкг`), "mcg"},
	{regexp.MustCompile(`мл`), "ml"},
	{regexp.MustCompile(`ЕД`), "U"},
}

// Dosage — дозировка врача на английском: частые слова переводятся, текст врача не меняется по смыслу.
func Dosage(l Lang, s string) string {
	if l != EN {
		return s
	}
	for _, w := range dosageWords {
		s = w.re.ReplaceAllString(s, w.en)
	}
	return s
}
