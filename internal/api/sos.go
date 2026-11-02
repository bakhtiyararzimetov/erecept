package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"erecept/internal/notifier"
)

// sos: пациент нажал «Мне плохо» — push всем родственникам. Повтор раньше 2 минут не шлёт ничего.
func (a *API) sos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pid, err := a.patientID(ctx, secretFrom(ctx))
	if err != nil {
		serverError(w, "пациент", err)
		return
	}
	// проверка и отметка времени — одним атомарным запросом
	var id int64
	err = a.DB.QueryRow(ctx, `UPDATE patients SET last_sos_at = now()
		WHERE id = $1 AND (last_sos_at IS NULL OR last_sos_at < now() - interval '2 minutes')
		RETURNING id`, pid).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"notified": 0, "throttled": true})
		return
	}
	if err != nil {
		serverError(w, "SOS", err)
		return
	}
	name, err := patientName(ctx, a.DB, pid)
	if err != nil {
		a.resetSOS(r, pid)
		serverError(w, "имя пациента", err)
		return
	}
	if name == "" {
		name = "Пациент"
	}
	rows, err := a.DB.Query(ctx, `SELECT push_token FROM relatives WHERE patient_id = $1 AND push_token <> ''`, pid)
	if err != nil {
		a.resetSOS(r, pid)
		serverError(w, "родственники", err)
		return
	}
	var msgs []notifier.Message
	for rows.Next() {
		var tok string
		if err := rows.Scan(&tok); err != nil {
			rows.Close()
			a.resetSOS(r, pid)
			serverError(w, "родственники", err)
			return
		}
		msgs = append(msgs, notifier.Message{
			To:    tok,
			Title: name + " нажал(а) «Мне плохо»",
			Body:  "Позвоните и узнайте, всё ли в порядке",
			Sound: "default",
			Data:  map[string]any{"type": "sos"},
		})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		a.resetSOS(r, pid)
		serverError(w, "родственники", err)
		return
	}
	if len(msgs) > 0 {
		if err := notifier.SendExpo(ctx, a.pushClient(), a.pushURL(), msgs); err != nil {
			logf("SOS push: %v", err)
			a.resetSOS(r, pid)
			writeError(w, http.StatusBadGateway, "Не удалось отправить уведомление родственникам")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"notified": len(msgs)})
}

// resetSOS снимает ограничение, чтобы пациент мог повторить нажатие после сбоя.
func (a *API) resetSOS(r *http.Request, pid int64) {
	// контекст запроса мог быть отменён — сбрасываем независимо от него
	if _, err := a.DB.Exec(context.WithoutCancel(r.Context()), `UPDATE patients SET last_sos_at = NULL WHERE id = $1`, pid); err != nil {
		logf("SOS сброс: %v", err)
	}
}
