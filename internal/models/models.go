// Package models: структуры, повторяющие таблицы базы. JSON-теги — для REST API.
package models

import "time"

// Когда пить относительно еды.
const (
	MealBefore = "before"
	MealAfter  = "after"
	MealDuring = "during"
	MealAny    = "any"
)

// MealLabel — подпись для интерфейса и текста напоминания.
func MealLabel(meal string) string {
	switch meal {
	case MealBefore:
		return "до еды"
	case MealAfter:
		return "после еды"
	case MealDuring:
		return "во время еды"
	default:
		return "независимо от еды"
	}
}

type Doctor struct {
	ID           int64     `json:"id"`
	Login        string    `json:"login"`
	PasswordHash string    `json:"-"`
	FullName     string    `json:"full_name"`
	Clinic       string    `json:"clinic"`
	CreatedAt    time.Time `json:"created_at"`
}

type Pharmacy struct {
	ID           int64     `json:"id"`
	Login        string    `json:"-"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Address      string    `json:"address"`
	Phone        string    `json:"phone"`
	WorkHours    string    `json:"work_hours"`
	Lat          float64   `json:"lat"`
	Lng          float64   `json:"lng"`
	CreatedAt    time.Time `json:"-"`
}

type Medicine struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	ActiveSubstance string `json:"active_substance"`
	Form            string `json:"form"`
}

type PharmacyItem struct {
	ID         int64     `json:"id"`
	PharmacyID int64     `json:"pharmacy_id"`
	MedicineID int64     `json:"medicine_id"`
	Price      int       `json:"price"` // тенге
	InStock    bool      `json:"in_stock"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Patient struct {
	ID           int64     `json:"id"`
	DeviceSecret string    `json:"-"`
	PushToken    string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Prescription struct {
	ID              int64      `json:"id"`
	Code            string     `json:"code"`
	DoctorID        int64      `json:"doctor_id"`
	PatientID       *int64     `json:"-"`
	PatientName     string     `json:"patient_name"`
	PatientBirth    time.Time  `json:"patient_birth"`
	Comment         string     `json:"comment"`
	WaterEveryHours int        `json:"water_every_hours"` // 0 = без напоминаний о воде
	WaterFrom       string     `json:"water_from"`        // "09:00"
	WaterTo         string     `json:"water_to"`          // "21:00"
	StartDate       *time.Time `json:"start_date"`        // nil, пока пациент не отсканировал
	ClaimedAt       *time.Time `json:"-"`                 // когда рецепт привязан к телефону
	CreatedAt       time.Time  `json:"created_at"`

	Items []PrescriptionItem `json:"items"`
}

type PrescriptionItem struct {
	ID             int64    `json:"id"`
	PrescriptionID int64    `json:"-"`
	Position       int      `json:"position"`
	MedicineName   string   `json:"medicine_name"`
	MedicineID     *int64   `json:"medicine_id"`
	Dosage         string   `json:"dosage"`
	TimesPerDay    int      `json:"times_per_day"`
	Days           int      `json:"days"`
	Meal           string   `json:"meal"`
	Times          []string `json:"times"` // ["08:00","20:00"]
	PhotoURL       string   `json:"photo_url"`
}

// Dose — один конкретный приём («Парацетамол, 14:00, день 3»).
type Dose struct {
	ID                 int64      `json:"id"`
	PrescriptionID     int64      `json:"prescription_id"`
	ItemID             int64      `json:"item_id"`
	ScheduledAt        time.Time  `json:"scheduled_at"`
	DayNumber          int        `json:"day_number"`
	TakenAt            *time.Time `json:"taken_at"`
	RelativeNotifiedAt *time.Time `json:"-"`
}

type Relative struct {
	ID           int64     `json:"id"`
	PatientID    int64     `json:"-"`
	DeviceSecret string    `json:"-"`
	Name         string    `json:"name"`
	PushToken    string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Invite struct {
	Code      string     `json:"code"`
	PatientID int64      `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"-"`
}
