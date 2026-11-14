package web

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"erecept/internal/i18n"
)

// PharmacyInfo — карточка аптеки.
type PharmacyInfo struct{ Name, Address, Phone, WorkHours string }

// PharmacyRow — строка таблицы позиций. Название, вещество и форма — на языке страницы.
type PharmacyRow struct {
	ID                    int64
	Name, Substance, Form string
	Price                 int
	InStock               bool
	Err                   string
}

// PharmacyStats — карточки над прайсом: всего позиций, в наличии, нет в наличии.
type PharmacyStats struct{ Total, InStock, OutOfStock int }

// itemCols — колонки строки прайса (pi — pharmacy_items, m — medicines); читает scanItem.
const itemCols = `pi.id, m.name, m.name_en, m.active_substance, m.substance_en, m.form, pi.price, pi.in_stock`

// scanItem читает строку itemCols и переводит справочные поля на язык l.
func scanItem(row pgx.Row, l i18n.Lang) (PharmacyRow, error) {
	var it PharmacyRow
	var nameEN, substanceEN string
	err := row.Scan(&it.ID, &it.Name, &nameEN, &it.Substance, &substanceEN, &it.Form, &it.Price, &it.InStock)
	it.Name, it.Substance, it.Form = l.Pick(it.Name, nameEN), l.Pick(it.Substance, substanceEN), l.Form(it.Form)
	return it, err
}

// ItemInput — форма «Добавить лекарство», значения как ввёл пользователь.
type ItemInput struct {
	Name, Substance, Form, Price string
	InStock                      bool
	BadName, BadPrice            bool
}

// money: 1250 → «1 250 ₸» (неразрывные пробелы).
func money(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(" ")
		}
		b.WriteRune(c)
	}
	return b.String() + " ₸"
}

type pageExtra struct {
	Notice, FormTitle string
	Errors            []string
	Form              ItemInput
	CSVDone           bool
	CSVOK             int
	CSVErrors         []string
	CSVMoreErrors     int
	CSVTotalErrors    int
}

func (w *Web) pharmacyPage(rw http.ResponseWriter, r *http.Request, status int, x pageExtra) {
	ctx := r.Context()
	l := i18n.FromContext(ctx)
	pid := currentUser(r).ID
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	data := map[string]any{
		"Q": q, "Medicines": w.medicineNames(ctx),
		"Notice": x.Notice, "FormTitle": x.FormTitle, "Errors": x.Errors, "Form": x.Form,
		"CSVDone": x.CSVDone, "CSVOK": x.CSVOK, "CSVErrors": x.CSVErrors, "CSVMoreErrors": x.CSVMoreErrors, "CSVTotalErrors": x.CSVTotalErrors,
	}
	var p PharmacyInfo
	if err := w.DB.QueryRow(ctx, `SELECT name, address, phone, work_hours FROM pharmacies WHERE id=$1`, pid).Scan(&p.Name, &p.Address, &p.Phone, &p.WorkHours); err != nil {
		log.Printf("аптека %d: %v", pid, err)
		http.Error(rw, tr(r, "Не удалось загрузить данные аптеки, попробуйте ещё раз"), http.StatusInternalServerError)
		return
	}
	p.Name, p.Address, p.WorkHours = l.Place(p.Name), l.Place(p.Address), l.Hours(p.WorkHours)
	data["Pharmacy"] = p
	// Поиск — по русскому и английскому названию; порядок — по названию на языке страницы.
	rows, err := w.DB.Query(ctx, `
		SELECT `+itemCols+`
		FROM pharmacy_items pi JOIN medicines m ON m.id = pi.medicine_id
		WHERE pi.pharmacy_id = $1 AND ($2 = '' OR m.name ILIKE '%' || $2 || '%' OR m.name_en ILIKE '%' || $2 || '%')
		ORDER BY lower(CASE WHEN $3 = 'en' AND m.name_en <> '' THEN m.name_en ELSE m.name END)`, pid, escapeLike(q), string(l))
	if err != nil {
		log.Printf("позиции аптеки %d: %v", pid, err)
		http.Error(rw, tr(r, "Не удалось загрузить позиции, попробуйте ещё раз"), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var items []PharmacyRow
	for rows.Next() {
		it, err := scanItem(rows, l)
		if err != nil {
			log.Printf("позиции аптеки %d: %v", pid, err)
			http.Error(rw, tr(r, "Не удалось загрузить позиции, попробуйте ещё раз"), http.StatusInternalServerError)
			return
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		log.Printf("позиции аптеки %d: %v", pid, err)
		http.Error(rw, tr(r, "Не удалось загрузить позиции, попробуйте ещё раз"), http.StatusInternalServerError)
		return
	}
	data["Items"] = items
	var st PharmacyStats
	if err := w.DB.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE in_stock), count(*) FILTER (WHERE NOT in_stock)
		FROM pharmacy_items WHERE pharmacy_id=$1`, pid).Scan(&st.Total, &st.InStock, &st.OutOfStock); err != nil {
		log.Printf("счётчики аптеки %d: %v", pid, err)
	}
	data["Stats"] = st
	w.renderStatus(rw, r, status, "pharmacy", data)
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (w *Web) pharmacyHome(rw http.ResponseWriter, r *http.Request) {
	w.pharmacyPage(rw, r, http.StatusOK, pageExtra{Form: ItemInput{InStock: true}})
}

func (w *Web) pharmacySample(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("Content-Type", "text/csv; charset=utf-8")
	rw.Header().Set("Content-Disposition", `attachment; filename="price-sample.csv"`)
	if i18n.FromContext(r.Context()) == i18n.EN {
		rw.Write([]byte(sampleCSVEN))
		return
	}
	rw.Write([]byte(sampleCSV))
}

// upsertItem находит лекарство в справочнике (по русскому или английскому названию)
// или создаёт его и обновляет позицию аптеки.
func upsertItem(ctx context.Context, tx pgx.Tx, pharmacyID int64, row CSVRow) error {
	var medID int64
	err := tx.QueryRow(ctx, `SELECT id FROM medicines WHERE lower(name) = lower($1) OR lower(name_en) = lower($1)
		ORDER BY lower(name) = lower($1) DESC, id LIMIT 1`, row.Name).Scan(&medID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO medicines (name, active_substance, form) VALUES ($1, $2, $3)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name RETURNING id`, row.Name, row.Substance, row.Form).Scan(&medID)
	} else if err == nil {
		_, err = tx.Exec(ctx, `UPDATE medicines SET
			active_substance = CASE WHEN active_substance = '' THEN $2 ELSE active_substance END,
			form = CASE WHEN form = '' THEN $3 ELSE form END WHERE id = $1`, medID, row.Substance, row.Form)
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO pharmacy_items (pharmacy_id, medicine_id, price, in_stock, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (pharmacy_id, medicine_id) DO UPDATE SET price = EXCLUDED.price, in_stock = EXCLUDED.in_stock, updated_at = now()`,
		pharmacyID, medID, row.Price, row.InStock)
	return err
}

func (w *Web) pharmacyAdd(rw http.ResponseWriter, r *http.Request) {
	in := ItemInput{
		Name:      strings.TrimSpace(r.PostFormValue("name")),
		Substance: strings.TrimSpace(r.PostFormValue("substance")),
		Form:      strings.TrimSpace(r.PostFormValue("form")),
		Price:     strings.TrimSpace(r.PostFormValue("price")),
		InStock:   r.PostFormValue("in_stock") != "",
	}
	var errs []string
	if in.Name == "" || len([]rune(in.Name)) > 200 {
		in.BadName = true
		errs = append(errs, tr(r, "Укажите название лекарства (до 200 символов)"))
	}
	price, err := parsePrice(in.Price)
	if err != nil {
		in.BadPrice = true
		errs = append(errs, tr(r, "Цена должна быть целым числом, например 1250"))
	}
	if len(errs) > 0 {
		w.pharmacyPage(rw, r, http.StatusUnprocessableEntity, pageExtra{Form: in, Errors: errs, FormTitle: tr(r, "Лекарство не сохранено. Исправьте:")})
		return
	}
	row := CSVRow{Name: in.Name, Substance: in.Substance, Form: in.Form, Price: price, InStock: in.InStock}
	if err := w.inTx(r.Context(), func(tx pgx.Tx) error { return upsertItem(r.Context(), tx, currentUser(r).ID, row) }); err != nil {
		log.Printf("добавление позиции: %v", err)
		in.BadName = false
		w.pharmacyPage(rw, r, http.StatusInternalServerError, pageExtra{Form: in, Errors: []string{tr(r, "Не удалось сохранить, попробуйте ещё раз")}, FormTitle: tr(r, "Ошибка")})
		return
	}
	w.pharmacyPage(rw, r, http.StatusOK, pageExtra{Form: ItemInput{InStock: true}, Notice: i18n.Tf(i18n.FromContext(r.Context()), "Сохранено: «%s» — %s", row.Name, money(price))})
}

func (w *Web) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

func (w *Web) pharmacyUpdate(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(rw, r)
		return
	}
	l := i18n.FromContext(r.Context())
	price, perr := parsePrice(r.PostFormValue("price"))
	var it PharmacyRow
	if perr != nil {
		it, err = scanItem(w.DB.QueryRow(r.Context(), `SELECT `+itemCols+`
			FROM pharmacy_items pi JOIN medicines m ON m.id = pi.medicine_id WHERE pi.id=$1 AND pi.pharmacy_id=$2`, id, currentUser(r).ID), l)
		it.Err = tr(r, "Цена должна быть целым числом")
	} else {
		it, err = scanItem(w.DB.QueryRow(r.Context(), `WITH pi AS (
				UPDATE pharmacy_items SET price=$3, in_stock=$4, updated_at=now() WHERE id=$1 AND pharmacy_id=$2 RETURNING id, medicine_id, price, in_stock)
			SELECT `+itemCols+` FROM pi JOIN medicines m ON m.id = pi.medicine_id`,
			id, currentUser(r).ID, price, r.PostFormValue("in_stock") != ""), l)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(rw, r)
		return
	}
	if err != nil {
		log.Printf("обновление позиции %d: %v", id, err)
		http.Error(rw, tr(r, "Не удалось сохранить, попробуйте ещё раз"), http.StatusInternalServerError)
		return
	}
	if !isHTMX(r) {
		if it.Err != "" {
			w.pharmacyPage(rw, r, http.StatusUnprocessableEntity, pageExtra{Errors: []string{it.Name + ": " + it.Err}, FormTitle: tr(r, "Цена не сохранена."), Form: ItemInput{InStock: true}})
			return
		}
		http.Redirect(rw, r, "/pharmacy", http.StatusSeeOther)
		return
	}
	var buf strings.Builder
	if err := w.mustTpl(r, "pharmacy").ExecuteTemplate(&buf, "pharmacy_row", it); err != nil {
		log.Printf("строка позиции: %v", err)
		http.Error(rw, tr(r, "Не удалось показать строку"), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.Write([]byte(buf.String()))
}

func (w *Web) pharmacyDelete(rw http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(rw, r)
		return
	}
	tag, err := w.DB.Exec(r.Context(), `DELETE FROM pharmacy_items WHERE id=$1 AND pharmacy_id=$2`, id, currentUser(r).ID)
	if err != nil {
		log.Printf("удаление позиции %d: %v", id, err)
		http.Error(rw, tr(r, "Не удалось удалить, попробуйте ещё раз"), http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		http.NotFound(rw, r)
		return
	}
	if isHTMX(r) {
		rw.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(rw, r, "/pharmacy", http.StatusSeeOther)
}

func (w *Web) pharmacyCSV(rw http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(rw, r.Body, maxCSVBytes+1<<20)
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		status, msg := http.StatusBadRequest, tr(r, "Не удалось прочитать файл")
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			status, msg = http.StatusRequestEntityTooLarge, tr(r, errTooBig)
		} else {
			log.Printf("загрузка CSV: разбор формы: %v", err)
		}
		w.pharmacyPage(rw, r, status, pageExtra{Form: ItemInput{InStock: true}, FormTitle: tr(r, "Файл не загружен."), Errors: []string{msg}})
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		w.pharmacyPage(rw, r, http.StatusUnprocessableEntity, pageExtra{Form: ItemInput{InStock: true}, FormTitle: tr(r, "Файл не загружен."), Errors: []string{tr(r, "Выберите файл CSV")}})
		return
	}
	defer f.Close()
	if hdr.Size > maxCSVBytes {
		w.pharmacyPage(rw, r, http.StatusRequestEntityTooLarge, pageExtra{Form: ItemInput{InStock: true}, FormTitle: tr(r, "Файл не загружен."), Errors: []string{tr(r, errTooBig)}})
		return
	}
	rows, errs, fatal := parseCSV(f, i18n.FromContext(r.Context()))
	if fatal != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(fatal, errCSVTooBig) {
			status = http.StatusRequestEntityTooLarge
		}
		w.pharmacyPage(rw, r, status, pageExtra{Form: ItemInput{InStock: true}, FormTitle: tr(r, "Файл не загружен."), Errors: errs})
		return
	}
	pid := currentUser(r).ID
	err = w.inTx(r.Context(), func(tx pgx.Tx) error {
		for _, row := range rows {
			if err := upsertItem(r.Context(), tx, pid, row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("загрузка CSV: %v", err)
		w.pharmacyPage(rw, r, http.StatusInternalServerError, pageExtra{Form: ItemInput{InStock: true}, FormTitle: tr(r, "Ошибка."), Errors: []string{tr(r, "Не удалось сохранить данные, ничего не загружено. Попробуйте ещё раз")}})
		return
	}
	x := pageExtra{Form: ItemInput{InStock: true}, CSVDone: true, CSVOK: len(rows), CSVErrors: errs, CSVTotalErrors: len(errs)}
	if len(errs) > 50 {
		x.CSVErrors, x.CSVMoreErrors = errs[:50], len(errs)-50
	}
	w.pharmacyPage(rw, r, http.StatusOK, x)
}
