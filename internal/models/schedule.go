package models

import (
	"sort"
	"strconv"
	"time"
)

// ValidTime проверяет время приёма в формате «HH:MM» (00:00–23:59).
func ValidTime(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	h, _ := strconv.Atoi(s[0:2])
	m, _ := strconv.Atoi(s[3:5])
	return h <= 23 && m <= 59
}

// GenerateDoses раскладывает назначение на конкретные приёмы.
// start — полночь первого дня в нужном часовом поясе. Для каждого дня и каждого
// времени из item.Times (по возрастанию) получается один приём.
func GenerateDoses(item PrescriptionItem, start time.Time) []Dose {
	times := append([]string(nil), item.Times...)
	sort.Strings(times)

	var doses []Dose
	for d := 0; d < item.Days; d++ {
		for _, t := range times {
			if !ValidTime(t) {
				continue
			}
			h, _ := strconv.Atoi(t[0:2])
			m, _ := strconv.Atoi(t[3:5])
			at := time.Date(start.Year(), start.Month(), start.Day()+d, h, m, 0, 0, start.Location())
			doses = append(doses, Dose{
				ItemID:         item.ID,
				PrescriptionID: item.PrescriptionID,
				ScheduledAt:    at,
				DayNumber:      d + 1,
			})
		}
	}
	return doses
}
