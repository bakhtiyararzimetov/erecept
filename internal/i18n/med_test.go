package i18n

import "testing"

func TestMedHelpers(t *testing.T) {
	if got := EN.Form("капсулы 20 мг №30"); got != "capsules 20 mg, 30 pcs" {
		t.Errorf("Form: %q", got)
	}
	if got := RU.Form("капсулы 20 мг №30"); got != "капсулы 20 мг №30" {
		t.Errorf("RU Form: %q", got)
	}
	if got := EN.Place("Аптека «Береке»"); got != "Pharmacy «Береке»" {
		t.Errorf("Place: %q", got)
	}
	if got := EN.Hours("Круглосуточно"); got != "24 hours" {
		t.Errorf("Hours: %q", got)
	}
	if EN.Pick("Омез", "Omez") != "Omez" || EN.Pick("Тестин", "") != "Тестин" || RU.Pick("Омез", "Omez") != "Омез" {
		t.Error("Pick")
	}
}
