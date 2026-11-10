package api

import (
	"context"
	"encoding/json"
	"erecept/internal/i18n"
	"time"

	"erecept/internal/explain"
	"erecept/internal/models"
)

type ItemDTO struct {
	ID           int64    `json:"id"`
	MedicineName string   `json:"medicine_name"`
	MedicineID   *int64   `json:"medicine_id"`
	Dosage       string   `json:"dosage"`
	TimesPerDay  int      `json:"times_per_day"`
	Days         int      `json:"days"`
	Meal         string   `json:"meal"`
	MealLabel    string   `json:"meal_label"`
	Times        []string `json:"times"`
	PhotoURL     string   `json:"photo_url"`
	// Explanation — ИИ-памятка о лекарстве (internal/explain) с правилами безопасности; null, если её нет.
	Explanation json.RawMessage `json:"explanation"`
}

type PrescriptionDTO struct {
	ID              int64     `json:"id"`
	Code            string    `json:"code"`
	PatientName     string    `json:"patient_name"`
	PatientBirth    string    `json:"patient_birth"`
	DoctorName      string    `json:"doctor_name"`
	Comment         string    `json:"comment"`
	WaterEveryHours int       `json:"water_every_hours"`
	WaterFrom       string    `json:"water_from"`
	WaterTo         string    `json:"water_to"`
	StartDate       *string   `json:"start_date"`
	Items           []ItemDTO `json:"items"`
}

type DoseDTO struct {
	ID          int64   `json:"id"`
	ItemID      int64   `json:"item_id"`
	ScheduledAt string  `json:"scheduled_at"`
	DayNumber   int     `json:"day_number"`
	TotalDays   int     `json:"total_days"`
	TakenAt     *string `json:"taken_at"`
}

type PrescriptionWithDoses struct {
	Prescription PrescriptionDTO `json:"prescription"`
	Doses        []DoseDTO       `json:"doses"`
}

func fmtTime(t time.Time) string { return t.In(time.Local).Format(time.RFC3339) }

// loadPrescription читает рецепт с позициями по условию where (например "p.code = $1").
func (a *API) loadPrescription(ctx context.Context, where string, arg any) (*PrescriptionDTO, error) {
	var p PrescriptionDTO
	var birth time.Time
	var start *time.Time
	err := a.DB.QueryRow(ctx, `SELECT p.id, p.code, p.patient_name, p.patient_birth, d.full_name, p.comment,
		p.water_every_hours, p.water_from, p.water_to, p.start_date
		FROM prescriptions p JOIN doctors d ON d.id = p.doctor_id WHERE `+where, arg).
		Scan(&p.ID, &p.Code, &p.PatientName, &birth, &p.DoctorName, &p.Comment,
			&p.WaterEveryHours, &p.WaterFrom, &p.WaterTo, &start)
	if err != nil {
		return nil, err
	}
	p.PatientBirth = birth.Format("2006-01-02")
	if start != nil {
		s := start.Format("2006-01-02")
		p.StartDate = &s
	}
	// Английское приложение: название из справочника и перевод памятки, если они есть.
	l := i18n.FromContext(ctx)
	name, data := "i.medicine_name", "e.data"
	if l == i18n.EN {
		name, data = "COALESCE(NULLIF(m.name_en, ''), i.medicine_name)", "COALESCE(e.data_en, e.data)"
	}
	rows, err := a.DB.Query(ctx, `SELECT i.id, `+name+`, i.medicine_id, i.dosage, i.times_per_day, i.days, i.meal, i.times, i.photo_url, `+data+`, e.data_en IS NOT NULL, COALESCE(m.active_substance, '')
		FROM prescription_items i LEFT JOIN medicine_explanations e ON e.medicine_id = i.medicine_id
		LEFT JOIN medicines m ON m.id = i.medicine_id
		WHERE i.prescription_id = $1 ORDER BY i.position, i.id`, p.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	p.Items = []ItemDTO{}
	for rows.Next() {
		var it ItemDTO
		var substance string
		var hasEN bool
		if err := rows.Scan(&it.ID, &it.MedicineName, &it.MedicineID, &it.Dosage, &it.TimesPerDay,
			&it.Days, &it.Meal, &it.Times, &it.PhotoURL, &it.Explanation, &hasEN, &substance); err != nil {
			return nil, err
		}
		if l == i18n.EN && hasEN {
			it.Explanation = explain.ServeJSONEN(it.Explanation, substance)
		} else {
			it.Explanation = explain.ServeJSON(it.Explanation, substance)
		}
		it.MealLabel = i18n.T(l, models.MealLabel(it.Meal))
		p.Items = append(p.Items, it)
	}
	return &p, rows.Err()
}

func (a *API) loadDoses(ctx context.Context, prescriptionID int64) ([]DoseDTO, error) {
	rows, err := a.DB.Query(ctx, `SELECT d.id, d.item_id, d.scheduled_at, d.day_number, i.days, d.taken_at
		FROM doses d JOIN prescription_items i ON i.id = d.item_id
		WHERE d.prescription_id = $1 ORDER BY d.scheduled_at, d.id`, prescriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	doses := []DoseDTO{}
	for rows.Next() {
		var d DoseDTO
		var at time.Time
		var taken *time.Time
		if err := rows.Scan(&d.ID, &d.ItemID, &at, &d.DayNumber, &d.TotalDays, &taken); err != nil {
			return nil, err
		}
		d.ScheduledAt = fmtTime(at)
		if taken != nil {
			s := fmtTime(*taken)
			d.TakenAt = &s
		}
		doses = append(doses, d)
	}
	return doses, rows.Err()
}
