package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"erecept/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ItemForm — строка лекарства в форме; значения хранятся как ввёл врач.
type ItemForm struct {
	I           int
	Name        string
	Dosage      string
	TimesPerDay int
	Days        string
	Meal        string
	Times       string
	Bad         bool // в строке есть ошибка
}

// RxForm — вся форма нового рецепта.
type RxForm struct {
	PatientName  string
	PatientBirth string
	Comment      string
	WaterEvery   int
	WaterFrom    string
	WaterTo      string
	Items        []ItemForm
	BadFields    map[string]bool
}

const (
	maxItemIndex = 999 // индекс строки лекарства в форме: 0..maxItemIndex
	maxItems     = 20
)

var defaultTimes = map[int]string{1: "08:00", 2: "08:00, 20:00", 3: "08:00, 14:00, 20:00", 4: "08:00, 12:00, 16:00, 20:00"}

// NextIndex — свободный индекс для новой строки (после удалений индексы идут с пропусками).
func (f RxForm) NextIndex() int {
	n := 0
	for _, it := range f.Items {
		if it.I >= n {
			n = it.I + 1
		}
	}
	return n
}

// TPDOptions — варианты «раз в день»; введённое значение вне списка добавляется, чтобы не сбрасываться.
func (it ItemForm) TPDOptions() []int {
	opts := []int{1, 2, 3, 4, 5, 6}
	if it.TimesPerDay > 6 && it.TimesPerDay <= 12 {
		opts = append(opts, it.TimesPerDay)
	}
	return opts
}

func newItemForm(i int) ItemForm {
	return ItemForm{I: i, TimesPerDay: 1, Days: "7", Meal: models.MealAfter, Times: defaultTimes[1]}
}

func emptyForm() RxForm {
	return RxForm{WaterFrom: "09:00", WaterTo: "21:00", Items: []ItemForm{newItemForm(0)}}
}

func (w *Web) medicineNames(ctx context.Context) []string {
	rows, err := w.DB.Query(ctx, `SELECT name FROM medicines ORDER BY name`)
	if err != nil {
		log.Printf("справочник лекарств: %v", err)
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			names = append(names, n)
		}
	}
	return names
}

func (w *Web) renderForm(rw http.ResponseWriter, r *http.Request, status int, f RxForm, errs []string) {
	w.renderStatus(rw, r, status, "doctor_new", map[string]any{
		"Form": f, "Errors": errs, "Medicines": w.medicineNames(r.Context()),
	})
}

func (w *Web) doctorNew(rw http.ResponseWriter, r *http.Request) {
	w.renderForm(rw, r, http.StatusOK, emptyForm(), nil)
}

// itemRow — htmx: одна новая строка таблицы лекарств, без шапки страницы.
func (w *Web) itemRow(rw http.ResponseWriter, r *http.Request) {
	i, err := strconv.Atoi(r.URL.Query().Get("i"))
	if err != nil || i < 0 || i > maxItemIndex {
		i = 0
	}
	t := w.pages["doctor_new"]
	var buf strings.Builder
	if err := t.ExecuteTemplate(&buf, "item_row", newItemForm(i)); err != nil {
		log.Printf("строка лекарства: %v", err)
		http.Error(rw, "Не удалось добавить строку", http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Write([]byte(buf.String()))
}

var itemKey = regexp.MustCompile(`^items\[(\d+)\]\.`)

var splitTimes = regexp.MustCompile(`[,;\s]+`)

// parseForm собирает форму из POST и проверяет её. Возвращает список ошибок на русском.
func parseForm(r *http.Request) (RxForm, []string) {
	f := RxForm{
		PatientName:  strings.TrimSpace(r.PostFormValue("patient_name")),
		PatientBirth: strings.TrimSpace(r.PostFormValue("patient_birth")),
		Comment:      strings.TrimSpace(r.PostFormValue("comment")),
		WaterFrom:    strings.TrimSpace(r.PostFormValue("water_from")),
		WaterTo:      strings.TrimSpace(r.PostFormValue("water_to")),
		BadFields:    map[string]bool{},
	}
	f.WaterEvery, _ = strconv.Atoi(r.PostFormValue("water_every_hours"))
	var errs []string
	bad := func(field, msg string) {
		f.BadFields[field] = true
		errs = append(errs, msg)
	}

	if f.PatientName == "" {
		bad("patient_name", "Укажите ФИО пациента")
	}
	if b, err := time.ParseInLocation("2006-01-02", f.PatientBirth, time.Local); err != nil {
		bad("patient_birth", "Укажите дату рождения пациента")
	} else if b.After(time.Now()) {
		bad("patient_birth", "Дата рождения не может быть в будущем")
	} else if b.Year() < 1900 {
		bad("patient_birth", "Проверьте дату рождения: год должен быть не раньше 1900")
	}
	if f.WaterEvery < 0 || f.WaterEvery > 4 {
		bad("water_every_hours", "Напоминание о воде: выберите значение из списка")
		f.WaterEvery = 0
	}
	if f.WaterEvery > 0 {
		if !models.ValidTime(f.WaterFrom) || !models.ValidTime(f.WaterTo) {
			bad("water_from", "Напоминание о воде: время нужно в формате ЧЧ:ММ")
		} else if f.WaterFrom >= f.WaterTo {
			bad("water_from", "Напоминание о воде: «с» должно быть раньше, чем «до»")
		}
	}

	seen := map[int]bool{}
	var idx []int
	for k := range r.PostForm {
		if m := itemKey.FindStringSubmatch(k); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n <= maxItemIndex && !seen[n] {
				seen[n] = true
				idx = append(idx, n)
			}
		}
	}
	sort.Ints(idx)
	if len(idx) > maxItems {
		bad("items", fmt.Sprintf("Не больше %d лекарств в одном рецепте", maxItems))
		idx = idx[:maxItems]
	}
	for pos, i := range idx {
		get := func(field string) string {
			return strings.TrimSpace(r.PostFormValue(fmt.Sprintf("items[%d].%s", i, field)))
		}
		it := ItemForm{I: i, Name: get("name"), Dosage: get("dosage"), Days: get("days"), Meal: get("meal"), Times: get("times")}
		it.TimesPerDay, _ = strconv.Atoi(get("times_per_day"))
		pre := fmt.Sprintf("Лекарство %d: ", pos+1)
		rowBad := func(msg string) {
			it.Bad = true
			errs = append(errs, pre+msg)
		}
		if it.Name == "" {
			rowBad("Укажите название лекарства")
		}
		if it.Dosage == "" {
			rowBad("укажите дозировку")
		}
		if it.TimesPerDay < 1 || it.TimesPerDay > 12 {
			rowBad("укажите, сколько раз в день принимать")
		}
		if d, err := strconv.Atoi(it.Days); err != nil || d < 1 || d > 365 {
			rowBad("число дней должно быть от 1 до 365")
		}
		switch it.Meal {
		case models.MealBefore, models.MealAfter, models.MealDuring, models.MealAny:
		default:
			it.Meal = models.MealAny
			rowBad("выберите, когда принимать относительно еды")
		}
		if list := parseTimes(it.Times); it.Times == "" || len(list) == 0 {
			rowBad("укажите время приёма, например 08:00, 20:00")
		} else if t, ok := firstInvalidTime(list); !ok {
			rowBad(fmt.Sprintf("«%s» — время нужно в формате ЧЧ:ММ, например 08:00", t))
		} else if dup(list) {
			rowBad("время приёма повторяется")
		} else if it.TimesPerDay >= 1 && it.TimesPerDay <= 12 && len(list) != it.TimesPerDay {
			rowBad(fmt.Sprintf("число времён приёма должно совпадать с «раз в день» (указано %d, времён %d)", it.TimesPerDay, len(list)))
		}
		f.Items = append(f.Items, it)
	}
	if len(f.Items) == 0 {
		errs = append(errs, "Добавьте хотя бы одно лекарство")
		f.Items = []ItemForm{newItemForm(0)}
	}
	return f, errs
}

func parseTimes(s string) []string {
	var out []string
	for _, p := range splitTimes.Split(s, -1) {
		if p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out) // «20:00, 08:00» → «08:00, 20:00»
	return out
}

func firstInvalidTime(list []string) (string, bool) {
	for _, t := range list {
		if !models.ValidTime(t) {
			return t, false
		}
	}
	return "", true
}

func dup(list []string) bool {
	seen := map[string]bool{}
	for _, t := range list {
		if seen[t] {
			return true
		}
		seen[t] = true
	}
	return false
}

func (w *Web) createPrescription(rw http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.renderForm(rw, r, http.StatusUnprocessableEntity, emptyForm(), []string{"Не удалось прочитать форму, попробуйте ещё раз"})
		return
	}
	f, errs := parseForm(r)
	if len(errs) > 0 {
		w.renderForm(rw, r, http.StatusUnprocessableEntity, f, errs)
		return
	}
	id, err := w.insertPrescription(r.Context(), currentUser(r).ID, f)
	if err != nil {
		log.Printf("сохранение рецепта: %v", err)
		w.renderForm(rw, r, http.StatusInternalServerError, f, []string{"Не удалось сохранить рецепт, попробуйте ещё раз"})
		return
	}
	http.Redirect(rw, r, fmt.Sprintf("/doctor/prescriptions/%d", id), http.StatusSeeOther)
}

// insertPrescription сохраняет рецепт и все его лекарства в одной транзакции.
func (w *Web) insertPrescription(ctx context.Context, doctorID int64, f RxForm) (int64, error) {
	birth, _ := time.Parse("2006-01-02", f.PatientBirth)
	for attempt := 0; ; attempt++ {
		id, err := w.tryInsert(ctx, doctorID, f, birth, newCode())
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" && attempt < 3 { // код уже занят
			continue
		}
		return id, err
	}
}

func (w *Web) tryInsert(ctx context.Context, doctorID int64, f RxForm, birth time.Time, code string) (int64, error) {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	water := f.WaterEvery
	from, to := f.WaterFrom, f.WaterTo
	if water == 0 || !models.ValidTime(from) || !models.ValidTime(to) {
		from, to = "09:00", "21:00"
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO prescriptions (code, doctor_id, patient_name, patient_birth, comment, water_every_hours, water_from, water_to)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		code, doctorID, f.PatientName, birth, f.Comment, water, from, to).Scan(&id)
	if err != nil {
		return 0, err
	}
	for pos, it := range f.Items {
		days, _ := strconv.Atoi(it.Days)
		_, err = tx.Exec(ctx, `INSERT INTO prescription_items (prescription_id, position, medicine_name, medicine_id, dosage, times_per_day, days, meal, times)
			VALUES ($1, $2, $3, (SELECT id FROM medicines WHERE lower(name) = lower($3) LIMIT 1), $4, $5, $6, $7, $8)`,
			id, pos, it.Name, it.Dosage, it.TimesPerDay, days, it.Meal, parseTimes(it.Times))
		if err != nil {
			return 0, err
		}
	}
	return id, tx.Commit(ctx)
}

// ownRx загружает рецепт из URL и проверяет, что он принадлежит вошедшему врачу.
// Чужой или несуществующий — 404 (ответ уже отправлен, ok=false).
func (w *Web) ownRx(rw http.ResponseWriter, r *http.Request) (RxView, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.notFound(rw, r)
		return RxView{}, false
	}
	p, err := w.loadPrescription(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.DoctorID != currentUser(r).ID) {
		w.notFound(rw, r)
		return RxView{}, false
	}
	var v RxView
	if err == nil {
		v, err = w.rxView(r.Context(), r, p)
	}
	if err != nil {
		log.Printf("рецепт %d: %v", id, err)
		http.Error(rw, "Не удалось показать рецепт, попробуйте ещё раз", http.StatusInternalServerError)
		return RxView{}, false
	}
	return v, true
}

func (w *Web) prescriptionPage(rw http.ResponseWriter, r *http.Request) {
	if v, ok := w.ownRx(rw, r); ok {
		w.render(rw, r, "doctor_rx", map[string]any{"Rx": v})
	}
}

func (w *Web) prescriptionPrint(rw http.ResponseWriter, r *http.Request) {
	if v, ok := w.ownRx(rw, r); ok {
		w.renderPlain(rw, "rx_print", map[string]any{"Rx": v})
	}
}

type listRow struct {
	ID          int64
	PatientName string
	Date        string
	Started     bool
	Percent     string // «80%» или «—»
}

func (w *Web) prescriptionList(rw http.ResponseWriter, r *http.Request) {
	rows, err := w.DB.Query(r.Context(), `
		SELECT p.id, p.patient_name, p.created_at, p.start_date IS NOT NULL,
		       count(d.id) FILTER (WHERE d.scheduled_at <= now()),
		       count(d.id) FILTER (WHERE d.scheduled_at <= now() AND d.taken_at IS NOT NULL)
		FROM prescriptions p LEFT JOIN doses d ON d.prescription_id = p.id
		WHERE p.doctor_id = $1
		GROUP BY p.id ORDER BY p.created_at DESC, p.id DESC`, currentUser(r).ID)
	if err != nil {
		log.Printf("список рецептов: %v", err)
		http.Error(rw, "Не удалось загрузить рецепты, попробуйте ещё раз", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []listRow
	for rows.Next() {
		var lr listRow
		var created time.Time
		var due, taken int
		if err := rows.Scan(&lr.ID, &lr.PatientName, &created, &lr.Started, &due, &taken); err != nil {
			log.Printf("список рецептов: %v", err)
			http.Error(rw, "Не удалось загрузить рецепты, попробуйте ещё раз", http.StatusInternalServerError)
			return
		}
		lr.Date = created.In(time.Local).Format("02.01.2006")
		lr.Percent = "—"
		if due > 0 {
			lr.Percent = fmt.Sprintf("%d%%", (taken*100+due/2)/due)
		}
		list = append(list, lr)
	}
	if err := rows.Err(); err != nil {
		log.Printf("список рецептов: %v", err)
		http.Error(rw, "Не удалось загрузить рецепты, попробуйте ещё раз", http.StatusInternalServerError)
		return
	}
	w.render(rw, r, "doctor_list", map[string]any{"Rows": list})
}
