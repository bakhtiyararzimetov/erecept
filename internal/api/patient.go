package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"erecept/internal/models"
)

var logf = log.Printf

func (a *API) getPrescription(w http.ResponseWriter, r *http.Request) {
	p, err := a.loadPrescription(r.Context(), "p.code = $1", chi.URLParam(r, "code"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Рецепт не найден")
		return
	}
	if err != nil {
		serverError(w, "рецепт", err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// claim привязывает рецепт к телефону и создаёт приёмы. Повторный вызов (в том числе
// с другого телефона) приёмы не дублирует и отметки не трогает.
func (a *API) claim(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := chi.URLParam(r, "code")
	pid, err := a.patientID(ctx, secretFrom(ctx))
	if err != nil {
		serverError(w, "пациент", err)
		return
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		serverError(w, "транзакция", err)
		return
	}
	defer tx.Rollback(ctx)

	now := time.Now().In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var rxID int64
	var start time.Time
	err = tx.QueryRow(ctx, `UPDATE prescriptions SET patient_id = $1,
			start_date = COALESCE(start_date, $2::date), claimed_at = COALESCE(claimed_at, now())
		WHERE code = $3 RETURNING id, start_date`, pid, today, code).Scan(&rxID, &start)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "Рецепт не найден")
		return
	}
	if err != nil {
		serverError(w, "привязка рецепта", err)
		return
	}
	startLocal := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.Local)

	rows, err := tx.Query(ctx, `SELECT id, days, times FROM prescription_items WHERE prescription_id = $1`, rxID)
	if err != nil {
		serverError(w, "позиции", err)
		return
	}
	var items []models.PrescriptionItem
	for rows.Next() {
		it := models.PrescriptionItem{PrescriptionID: rxID}
		if err := rows.Scan(&it.ID, &it.Days, &it.Times); err != nil {
			rows.Close()
			serverError(w, "позиции", err)
			return
		}
		items = append(items, it)
	}
	rows.Close()
	if rows.Err() != nil {
		serverError(w, "позиции", rows.Err())
		return
	}
	for _, it := range items {
		for _, d := range models.GenerateDoses(it, startLocal) {
			if _, err := tx.Exec(ctx, `INSERT INTO doses (prescription_id, item_id, scheduled_at, day_number)
				VALUES ($1, $2, $3, $4) ON CONFLICT (item_id, scheduled_at) DO NOTHING`,
				rxID, it.ID, d.ScheduledAt, d.DayNumber); err != nil {
				serverError(w, "приёмы", err)
				return
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "коммит", err)
		return
	}
	a.writePrescriptionWithDoses(w, r, rxID)
}

func (a *API) writePrescriptionWithDoses(w http.ResponseWriter, r *http.Request, rxID int64) {
	p, err := a.loadPrescription(r.Context(), "p.id = $1", rxID)
	if err != nil {
		serverError(w, "рецепт", err)
		return
	}
	doses, err := a.loadDoses(r.Context(), rxID)
	if err != nil {
		serverError(w, "приёмы", err)
		return
	}
	writeJSON(w, http.StatusOK, PrescriptionWithDoses{*p, doses})
}

func (a *API) myPrescriptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pid, err := a.patientID(ctx, secretFrom(ctx))
	if err != nil {
		serverError(w, "пациент", err)
		return
	}
	rows, err := a.DB.Query(ctx, `SELECT id FROM prescriptions WHERE patient_id = $1 ORDER BY claimed_at DESC NULLS LAST, id DESC`, pid)
	if err != nil {
		serverError(w, "рецепты", err)
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			serverError(w, "рецепты", err)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	out := []PrescriptionWithDoses{}
	for _, id := range ids {
		p, err := a.loadPrescription(ctx, "p.id = $1", id)
		if err != nil {
			serverError(w, "рецепт", err)
			return
		}
		doses, err := a.loadDoses(ctx, id)
		if err != nil {
			serverError(w, "приёмы", err)
			return
		}
		out = append(out, PrescriptionWithDoses{*p, doses})
	}
	writeJSON(w, http.StatusOK, out)
}

// syncDoses принимает отметки «Выпил», сделанные офлайн. Идемпотентно:
// уже отмеченные приёмы не перезаписываются, чужие и неизвестные пропускаются.
func (a *API) syncDoses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Marks []struct {
			DoseID  int64     `json:"dose_id"`
			TakenAt time.Time `json:"taken_at"`
		} `json:"marks"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	pid, err := a.patientID(ctx, secretFrom(ctx))
	if err != nil {
		serverError(w, "пациент", err)
		return
	}
	updated := 0
	for _, m := range req.Marks {
		tag, err := a.DB.Exec(ctx, `UPDATE doses d SET taken_at = $1
			FROM prescriptions p WHERE d.id = $2 AND p.id = d.prescription_id
			AND p.patient_id = $3 AND d.taken_at IS NULL`, m.TakenAt, m.DoseID, pid)
		if err != nil {
			serverError(w, "отметка", err)
			return
		}
		updated += int(tag.RowsAffected())
	}
	writeJSON(w, http.StatusOK, map[string]int{"updated": updated})
}

func (a *API) pushToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil || req.Token == "" {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	secret := secretFrom(ctx)
	t1, err := a.DB.Exec(ctx, `UPDATE patients SET push_token = $1 WHERE device_secret = $2`, req.Token, secret)
	if err != nil {
		serverError(w, "push-токен", err)
		return
	}
	t2, err := a.DB.Exec(ctx, `UPDATE relatives SET push_token = $1 WHERE device_secret = $2`, req.Token, secret)
	if err != nil {
		serverError(w, "push-токен", err)
		return
	}
	if t1.RowsAffected()+t2.RowsAffected() == 0 {
		// первое обращение приложения: создаём пациента с токеном
		if _, err := a.DB.Exec(ctx, `INSERT INTO patients (device_secret, push_token) VALUES ($1, $2)
			ON CONFLICT (device_secret) DO UPDATE SET push_token = EXCLUDED.push_token`, secret, req.Token); err != nil {
			serverError(w, "push-токен", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
