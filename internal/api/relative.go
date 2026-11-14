package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"erecept/internal/i18n"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	s := n.String()
	return strings.Repeat("0", 6-len(s)) + s, nil
}

func (a *API) createInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pid, err := a.patientID(ctx, secretFrom(ctx))
	if err != nil {
		serverError(w, "пациент", err)
		return
	}
	expires := time.Now().Add(24 * time.Hour)
	for attempt := 0; attempt < 10; attempt++ {
		code, err := randomCode()
		if err != nil {
			serverError(w, "код", err)
			return
		}
		// занятым считается только действующий код: просроченные и использованные можно переиспользовать
		_, err = a.DB.Exec(ctx, `DELETE FROM invites WHERE code = $1 AND (used_at IS NOT NULL OR expires_at <= now())`, code)
		if err != nil {
			serverError(w, "приглашение", err)
			return
		}
		_, err = a.DB.Exec(ctx, `INSERT INTO invites (code, patient_id, expires_at) VALUES ($1, $2, $3)`, code, pid, expires)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		if err != nil {
			serverError(w, "приглашение", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"code": code, "expires_at": fmtTime(expires)})
		return
	}
	serverError(w, "приглашение", errors.New("не удалось подобрать код"))
}

func (a *API) joinRelative(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "неверный запрос")
		return
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		serverError(w, "транзакция", err)
		return
	}
	defer tx.Rollback(ctx)

	var pid int64
	err = tx.QueryRow(ctx, `UPDATE invites SET used_at = now()
		WHERE code = $1 AND used_at IS NULL AND expires_at > now() RETURNING patient_id`, strings.TrimSpace(req.Code)).Scan(&pid)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusBadRequest, "Код неверный или истёк")
		return
	}
	if err != nil {
		serverError(w, "приглашение", err)
		return
	}
	// одно устройство = один родственник: новый код перепривязывает его к новому пациенту
	if _, err := tx.Exec(ctx, `INSERT INTO relatives (patient_id, device_secret, name) VALUES ($1, $2, $3)
		ON CONFLICT (device_secret) DO UPDATE SET patient_id = EXCLUDED.patient_id, name = EXCLUDED.name`,
		pid, secretFrom(ctx), strings.TrimSpace(req.Name)); err != nil {
		serverError(w, "родственник", err)
		return
	}
	name, err := patientName(ctx, tx, pid)
	if err != nil {
		serverError(w, "имя пациента", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "коммит", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"patient_name": name})
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// patientName — имя из последнего рецепта пациента (пусто, если рецептов нет).
func patientName(ctx context.Context, q querier, pid int64) (string, error) {
	var name string
	err := q.QueryRow(ctx, `SELECT patient_name FROM prescriptions WHERE patient_id = $1
		ORDER BY claimed_at DESC NULLS LAST, id DESC LIMIT 1`, pid).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return i18n.Person(i18n.FromContext(ctx), name), err
}

type missedDTO struct {
	MedicineName string `json:"medicine_name"`
	ScheduledAt  string `json:"scheduled_at"`
}

type overviewDTO struct {
	PatientName  string      `json:"patient_name"`
	Taken        int         `json:"taken"`
	Missed       int         `json:"missed"`
	Upcoming     int         `json:"upcoming"`
	RecentMissed []missedDTO `json:"recent_missed"`
}

func (a *API) relativeOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := a.DB.Query(ctx, `SELECT patient_id FROM relatives WHERE device_secret = $1`, secretFrom(ctx))
	if err != nil {
		serverError(w, "родственник", err)
		return
	}
	var pids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			serverError(w, "родственник", err)
			return
		}
		pids = append(pids, id)
	}
	rows.Close()

	out := []overviewDTO{}
	for _, pid := range pids {
		o := overviewDTO{RecentMissed: []missedDTO{}}
		if o.PatientName, err = patientName(ctx, a.DB, pid); err != nil {
			serverError(w, "имя пациента", err)
			return
		}
		err = a.DB.QueryRow(ctx, `SELECT
				count(*) FILTER (WHERE d.taken_at IS NOT NULL),
				count(*) FILTER (WHERE d.taken_at IS NULL AND d.scheduled_at <= now() - interval '60 minutes'),
				count(*) FILTER (WHERE d.taken_at IS NULL AND d.scheduled_at > now() - interval '60 minutes')
			FROM doses d JOIN prescriptions p ON p.id = d.prescription_id WHERE p.patient_id = $1`, pid).
			Scan(&o.Taken, &o.Missed, &o.Upcoming)
		if err != nil {
			serverError(w, "статистика", err)
			return
		}
		mrows, err := a.DB.Query(ctx, `SELECT i.medicine_name, d.scheduled_at
			FROM doses d JOIN prescriptions p ON p.id = d.prescription_id JOIN prescription_items i ON i.id = d.item_id
			WHERE p.patient_id = $1 AND d.taken_at IS NULL AND d.scheduled_at <= now() - interval '60 minutes'
			ORDER BY d.scheduled_at DESC LIMIT 5`, pid)
		if err != nil {
			serverError(w, "пропуски", err)
			return
		}
		for mrows.Next() {
			var m missedDTO
			var at time.Time
			if err := mrows.Scan(&m.MedicineName, &at); err != nil {
				mrows.Close()
				serverError(w, "пропуски", err)
				return
			}
			m.ScheduledAt = fmtTime(at)
			o.RecentMissed = append(o.RecentMissed, m)
		}
		mrows.Close()
		out = append(out, o)
	}
	writeJSON(w, http.StatusOK, map[string]any{"patients": out})
}
