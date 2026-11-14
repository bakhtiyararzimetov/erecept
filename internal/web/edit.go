package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"erecept/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Правка, отмена и копия рецепта. Править можно только до скана пациентом:
// после скана приёмы уже на телефоне, тогда врач отменяет рецепт и выписывает новый (копией).

var errLocked = errors.New("рецепт уже отсканирован или отменён")

// formFromRx — форма, заполненная из сохранённого рецепта.
func formFromRx(p models.Prescription) RxForm {
	f := RxForm{
		PatientName: p.PatientName, PatientBirth: p.PatientBirth.Format("02.01.2006"), Comment: p.Comment,
		WaterEvery: p.WaterEveryHours, WaterFrom: p.WaterFrom, WaterTo: p.WaterTo,
	}
	for i, it := range p.Items {
		f.Items = append(f.Items, ItemForm{I: i, Name: it.MedicineName, Dosage: it.Dosage, TimesPerDay: it.TimesPerDay,
			Days: strconv.Itoa(it.Days), Meal: it.Meal, Times: strings.Join(it.Times, ", ")})
	}
	if len(f.Items) == 0 {
		f.Items = []ItemForm{newItemForm(0)}
	}
	return f
}

// ownPrescription — рецепт вошедшего врача по {id} из URL; чужой или нет — 404 (ok=false).
func (w *Web) ownPrescription(rw http.ResponseWriter, r *http.Request) (models.Prescription, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		w.notFound(rw, r)
		return models.Prescription{}, false
	}
	p, err := w.loadPrescription(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && p.DoctorID != currentUser(r).ID) {
		w.notFound(rw, r)
		return p, false
	}
	if err != nil {
		log.Printf("рецепт %d: %v", id, err)
		http.Error(rw, tr(r, "Не удалось показать рецепт, попробуйте ещё раз"), http.StatusInternalServerError)
		return p, false
	}
	return p, true
}

func editable(p models.Prescription) bool { return p.StartDate == nil && p.CancelledAt == nil }

func (w *Web) editForm(rw http.ResponseWriter, r *http.Request) {
	p, ok := w.ownPrescription(rw, r)
	if !ok {
		return
	}
	if !editable(p) {
		http.Redirect(rw, r, fmt.Sprintf("/doctor/prescriptions/%d", p.ID), http.StatusSeeOther)
		return
	}
	w.renderEdit(rw, r, http.StatusOK, p.ID, formFromRx(p), nil)
}

func (w *Web) renderEdit(rw http.ResponseWriter, r *http.Request, status int, id int64, f RxForm, errs []string) {
	w.renderStatus(rw, r, status, "doctor_new", map[string]any{
		"Form": f, "Errors": errs, "Medicines": w.medicineNames(r.Context()), "EditID": id,
	})
}

func (w *Web) updatePrescription(rw http.ResponseWriter, r *http.Request) {
	p, ok := w.ownPrescription(rw, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		w.renderEdit(rw, r, http.StatusUnprocessableEntity, p.ID, formFromRx(p), []string{tr(r, "Не удалось прочитать форму, попробуйте ещё раз")})
		return
	}
	f, errs := parseForm(r)
	if !editable(p) {
		w.renderEdit(rw, r, http.StatusConflict, p.ID, f, []string{tr(r, "Пациент уже отсканировал рецепт или рецепт отменён — изменить его нельзя. Отмените рецепт и выпишите новый.")})
		return
	}
	if len(errs) > 0 {
		w.renderEdit(rw, r, http.StatusUnprocessableEntity, p.ID, f, errs)
		return
	}
	err := w.replacePrescription(r.Context(), p.ID, currentUser(r).ID, f)
	if errors.Is(err, errLocked) {
		w.renderEdit(rw, r, http.StatusConflict, p.ID, f, []string{tr(r, "Пациент уже отсканировал рецепт или рецепт отменён — изменить его нельзя. Отмените рецепт и выпишите новый.")})
		return
	}
	if err != nil {
		log.Printf("правка рецепта %d: %v", p.ID, err)
		w.renderEdit(rw, r, http.StatusInternalServerError, p.ID, f, []string{tr(r, "Не удалось сохранить рецепт, попробуйте ещё раз")})
		return
	}
	http.Redirect(rw, r, fmt.Sprintf("/doctor/prescriptions/%d", p.ID), http.StatusSeeOther)
}

// replacePrescription переписывает поля и лекарства, если рецепт ещё не отсканирован и не отменён.
func (w *Web) replacePrescription(ctx context.Context, id, doctorID int64, f RxForm) error {
	birth, _ := parseBirth(f.PatientBirth)
	tx, err := w.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	from, to := f.WaterFrom, f.WaterTo
	if f.WaterEvery == 0 || !models.ValidTime(from) || !models.ValidTime(to) {
		from, to = "09:00", "21:00"
	}
	tag, err := tx.Exec(ctx, `UPDATE prescriptions SET patient_name = $1, patient_birth = $2, comment = $3,
		water_every_hours = $4, water_from = $5, water_to = $6
		WHERE id = $7 AND doctor_id = $8 AND start_date IS NULL AND cancelled_at IS NULL`,
		f.PatientName, birth, f.Comment, f.WaterEvery, from, to, id, doctorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errLocked
	}
	if _, err := tx.Exec(ctx, `DELETE FROM prescription_items WHERE prescription_id = $1`, id); err != nil {
		return err
	}
	if err := insertItems(ctx, tx, id, f); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Web) cancelPrescription(rw http.ResponseWriter, r *http.Request) {
	p, ok := w.ownPrescription(rw, r)
	if !ok {
		return
	}
	if _, err := w.DB.Exec(r.Context(), `UPDATE prescriptions SET cancelled_at = now() WHERE id = $1 AND cancelled_at IS NULL`, p.ID); err != nil {
		log.Printf("отмена рецепта %d: %v", p.ID, err)
		http.Error(rw, tr(r, "Не удалось отменить рецепт, попробуйте ещё раз"), http.StatusInternalServerError)
		return
	}
	http.Redirect(rw, r, fmt.Sprintf("/doctor/prescriptions/%d", p.ID), http.StatusSeeOther)
}

// copyForm — форма нового рецепта из старого (?from=id); чужой или неверный id — пустая форма.
func (w *Web) copyForm(r *http.Request) (RxForm, bool) {
	id, err := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	if err != nil {
		return RxForm{}, false
	}
	p, err := w.loadPrescription(r.Context(), id)
	if err != nil || p.DoctorID != currentUser(r).ID {
		return RxForm{}, false
	}
	return formFromRx(p), true
}
