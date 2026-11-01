package models

import (
	"testing"
	"time"
)

func TestGenerateDoses(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Almaty")
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, loc)
	item := PrescriptionItem{ID: 7, PrescriptionID: 3, Days: 3, TimesPerDay: 2, Times: []string{"20:00", "08:00"}}
	ds := GenerateDoses(item, start)
	if len(ds) != 6 {
		t.Fatalf("len=%d", len(ds))
	}
	if !ds[0].ScheduledAt.Equal(time.Date(2026, 11, 1, 8, 0, 0, 0, loc)) || ds[0].DayNumber != 1 {
		t.Fatal(ds[0])
	}
	if ds[0].ItemID != 7 || ds[0].PrescriptionID != 3 {
		t.Fatal(ds[0])
	}
	if !ds[5].ScheduledAt.Equal(time.Date(2026, 11, 3, 20, 0, 0, 0, loc)) || ds[5].DayNumber != 3 {
		t.Fatal(ds[5])
	}
}

func TestValidTime(t *testing.T) {
	for _, s := range []string{"08:00", "23:59", "00:00"} {
		if !ValidTime(s) {
			t.Errorf("%q должно быть верным", s)
		}
	}
	for _, s := range []string{"8:00", "24:00", "12:60", "ab:cd", "", "08:00:00", "-1:00"} {
		if ValidTime(s) {
			t.Errorf("%q должно быть неверным", s)
		}
	}
}
