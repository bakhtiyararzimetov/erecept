package web

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"erecept/internal/explain"
	"erecept/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/skip2/go-qrcode"
)

// publicURL — адрес сайта для ссылки в QR: PUBLIC_URL, иначе RENDER_EXTERNAL_URL, иначе по заголовку Host.
func publicURL(r *http.Request) string {
	for _, k := range []string{"PUBLIC_URL", "RENDER_EXTERNAL_URL"} {
		if v := strings.TrimRight(strings.TrimSpace(os.Getenv(k)), "/"); v != "" {
			return v
		}
	}
	return "http://" + r.Host
}

// newCode — случайный код рецепта: 10 символов base32 в нижнем регистре.
func newCode() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

const prescriptionCols = `id, code, doctor_id, patient_name, patient_birth, comment,
	water_every_hours, water_from, water_to, start_date, claimed_at, created_at`

func (w *Web) loadPrescription(ctx context.Context, id int64) (models.Prescription, error) {
	return w.loadOne(ctx, `SELECT `+prescriptionCols+` FROM prescriptions WHERE id = $1`, id)
}

func (w *Web) loadPrescriptionByCode(ctx context.Context, code string) (models.Prescription, error) {
	return w.loadOne(ctx, `SELECT `+prescriptionCols+` FROM prescriptions WHERE code = $1`, code)
}

// loadOne читает рецепт и его лекарства (по position). Нет рецепта — pgx.ErrNoRows.
func (w *Web) loadOne(ctx context.Context, query string, arg any) (models.Prescription, error) {
	var p models.Prescription
	err := w.DB.QueryRow(ctx, query, arg).Scan(&p.ID, &p.Code, &p.DoctorID, &p.PatientName, &p.PatientBirth,
		&p.Comment, &p.WaterEveryHours, &p.WaterFrom, &p.WaterTo, &p.StartDate, &p.ClaimedAt, &p.CreatedAt)
	if err != nil {
		return p, err
	}
	rows, err := w.DB.Query(ctx, `SELECT id, position, medicine_name, medicine_id, dosage, times_per_day, days, meal, times
		FROM prescription_items WHERE prescription_id = $1 ORDER BY position, id`, p.ID)
	if err != nil {
		return p, err
	}
	defer rows.Close()
	for rows.Next() {
		var it models.PrescriptionItem
		if err := rows.Scan(&it.ID, &it.Position, &it.MedicineName, &it.MedicineID, &it.Dosage, &it.TimesPerDay, &it.Days, &it.Meal, &it.Times); err != nil {
			return p, err
		}
		it.PrescriptionID = p.ID
		p.Items = append(p.Items, it)
	}
	return p, rows.Err()
}

// ItemView — строка рецепта для чтения (печать, публичная страница).
type ItemView struct {
	Medicine, Dosage, How string
	Days                  int
	Explanation           *explain.Explanation // ИИ-памятка (врач и публичная страница); nil — нет
}

// RxView — рецепт, подготовленный к показу.
type RxView struct {
	models.Prescription
	DoctorName, Clinic string
	Birth, Date        string
	Items              []ItemView
	WaterText          string
	Link, QRPath       string
}

func timesWord(n int) string {
	if n >= 2 && n <= 4 {
		return "раза"
	}
	return "раз"
}

// howText: «2 раза в день, 08:00 и 20:00, после еды».
func howText(it models.PrescriptionItem) string {
	var t string
	switch n := len(it.Times); {
	case n == 0:
	case n == 1:
		t = it.Times[0]
	default:
		t = strings.Join(it.Times[:n-1], ", ") + " и " + it.Times[n-1]
	}
	return fmt.Sprintf("%d %s в день, %s, %s", it.TimesPerDay, timesWord(it.TimesPerDay), t, models.MealLabel(it.Meal))
}

func (w *Web) rxView(ctx context.Context, r *http.Request, p models.Prescription) (RxView, error) {
	v := RxView{Prescription: p}
	err := w.DB.QueryRow(ctx, `SELECT full_name, clinic FROM doctors WHERE id = $1`, p.DoctorID).Scan(&v.DoctorName, &v.Clinic)
	if err != nil {
		return v, err
	}
	v.Birth = p.PatientBirth.Format("02.01.2006")
	v.Date = p.CreatedAt.In(time.Local).Format("02.01.2006")
	expl := w.explanations(ctx, p.Items)
	for _, it := range p.Items {
		iv := ItemView{Medicine: it.MedicineName, Dosage: it.Dosage, How: howText(it), Days: it.Days}
		if it.MedicineID != nil {
			iv.Explanation = expl[*it.MedicineID]
		}
		v.Items = append(v.Items, iv)
	}
	if p.WaterEveryHours > 0 {
		v.WaterText = fmt.Sprintf("Пейте воду: каждые %d ч, с %s до %s", p.WaterEveryHours, p.WaterFrom, p.WaterTo)
	}
	v.Link = publicURL(r) + "/r/" + p.Code
	v.QRPath = "/qr/" + p.Code + ".png"
	return v, nil
}

// explanations — памятки к лекарствам рецепта по medicine_id. Памятка необязательна:
// при ошибке страница показывается без неё.
func (w *Web) explanations(ctx context.Context, items []models.PrescriptionItem) map[int64]*explain.Explanation {
	var ids []int64
	for _, it := range items {
		if it.MedicineID != nil {
			ids = append(ids, *it.MedicineID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := w.DB.Query(ctx, `SELECT e.medicine_id, e.data, m.active_substance
		FROM medicine_explanations e JOIN medicines m ON m.id = e.medicine_id WHERE e.medicine_id = ANY($1)`, ids)
	if err != nil {
		log.Printf("памятки рецепта: %v", err)
		return nil
	}
	defer rows.Close()
	out := map[int64]*explain.Explanation{}
	for rows.Next() {
		var id int64
		var ex explain.Explanation
		var substance string
		if err := rows.Scan(&id, &ex, &substance); err != nil {
			log.Printf("памятки рецепта: %v", err)
			continue
		}
		explain.ApplyRules(&ex, explain.IsHighRisk(substance, ""))
		out[id] = &ex
	}
	if err := rows.Err(); err != nil {
		log.Printf("памятки рецепта: %v", err)
	}
	return out
}

func (w *Web) notFound(rw http.ResponseWriter, r *http.Request) {
	w.renderStatus(rw, r, http.StatusNotFound, "placeholder", map[string]any{"Title": "Рецепт не найден", "Text": "Такого рецепта нет или он вам недоступен."})
}

// publicPage — читаемая страница рецепта без входа: то, что открывает любой сканер QR.
func (w *Web) publicPage(rw http.ResponseWriter, r *http.Request) {
	noIndex(rw)
	p, err := w.loadPrescriptionByCode(r.Context(), chi.URLParam(r, "code"))
	if errors.Is(err, pgx.ErrNoRows) {
		w.notFound(rw, r)
		return
	}
	if err != nil {
		log.Printf("публичный рецепт: %v", err)
		http.Error(rw, "Не удалось показать рецепт, попробуйте ещё раз", http.StatusInternalServerError)
		return
	}
	v, err := w.rxView(r.Context(), r, p)
	if err != nil {
		log.Printf("публичный рецепт: %v", err)
		http.Error(rw, "Не удалось показать рецепт, попробуйте ещё раз", http.StatusInternalServerError)
		return
	}
	w.renderPlain(rw, "rx_public", map[string]any{"Rx": v})
}

// qrPNG — PNG 512px со ссылкой на публичную страницу рецепта.
func (w *Web) qrPNG(rw http.ResponseWriter, r *http.Request) {
	noIndex(rw)
	code, ok := strings.CutSuffix(chi.URLParam(r, "file"), ".png")
	if !ok {
		http.NotFound(rw, r)
		return
	}
	var exists bool
	if err := w.DB.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM prescriptions WHERE code = $1)`, code).Scan(&exists); err != nil {
		log.Printf("QR: %v", err)
		http.Error(rw, "Не удалось создать QR-код", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.NotFound(rw, r)
		return
	}
	png, err := qrcode.Encode(publicURL(r)+"/r/"+code, qrcode.Medium, 512)
	if err != nil {
		log.Printf("QR: %v", err)
		http.Error(rw, "Не удалось создать QR-код", http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "image/png")
	rw.Write(png)
}

// noIndex: публичные страницы показывают ФИО и дату рождения, поисковикам они не нужны.
func noIndex(rw http.ResponseWriter) {
	rw.Header().Set("X-Robots-Tag", "noindex")
	rw.Header().Set("Cache-Control", "private, max-age=300")
}
