// Package notifier: фоновая проверка пропущенных приёмов и push через Expo Push API.
package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	missedAfter = 60 * time.Minute // через сколько после времени приёма считаем пропуск
	maxAge      = 3 * time.Hour    // более старые пропуски не рассылаем (сервер мог спать)
)

type Notifier struct {
	DB      *pgxpool.Pool
	PushURL string
	Client  *http.Client
	Now     func() time.Time
}

func New(pool *pgxpool.Pool) *Notifier {
	return &Notifier{
		DB:      pool,
		PushURL: "https://exp.host/--/api/v2/push/send",
		Client:  &http.Client{Timeout: 15 * time.Second},
		Now:     time.Now,
	}
}

// Run проверяет сразу и затем раз в минуту, пока не отменён ctx. Ошибки только пишет в лог.
func (n *Notifier) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		n.tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// tick — одна проверка; паника в ней пишется в лог и не останавливает цикл.
func (n *Notifier) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("уведомления: паника: %v", r)
		}
	}()
	if sent, err := n.CheckOnce(ctx); err != nil {
		log.Printf("уведомления: %v", err)
	} else if sent > 0 {
		log.Printf("уведомления: отправлено %d", sent)
	}
}

type message struct {
	To    string         `json:"to"`
	Title string         `json:"title"`
	Body  string         `json:"body"`
	Sound string         `json:"sound"`
	Data  map[string]any `json:"data"`
}

// CheckOnce шлёт push родственникам по свежим пропущенным приёмам и возвращает число сообщений.
func (n *Notifier) CheckOnce(ctx context.Context) (int, error) {
	now := n.Now()
	rows, err := n.DB.Query(ctx, `
		SELECT d.id, pr.patient_id, pr.patient_name, i.medicine_name, d.scheduled_at
		FROM doses d
		JOIN prescriptions pr ON pr.id = d.prescription_id
		JOIN prescription_items i ON i.id = d.item_id
		WHERE d.taken_at IS NULL AND d.relative_notified_at IS NULL
		  AND d.scheduled_at <= $1 AND d.scheduled_at > $2
		  AND pr.claimed_at IS NOT NULL AND d.scheduled_at >= pr.claimed_at
		ORDER BY d.scheduled_at`, now.Add(-missedAfter), now.Add(-maxAge))
	if err != nil {
		return 0, fmt.Errorf("поиск пропусков: %w", err)
	}
	type missed struct {
		id, patientID int64
		patient, med  string
		at            time.Time
	}
	var list []missed
	for rows.Next() {
		var m missed
		if err := rows.Scan(&m.id, &m.patientID, &m.patient, &m.med, &m.at); err != nil {
			rows.Close()
			return 0, fmt.Errorf("чтение пропусков: %w", err)
		}
		list = append(list, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("чтение пропусков: %w", err)
	}

	sent := 0
	var firstErr error
	for _, m := range list {
		tokens, err := n.tokens(ctx, m.patientID)
		if err == nil && len(tokens) > 0 {
			msgs := make([]message, 0, len(tokens))
			for _, tok := range tokens {
				msgs = append(msgs, message{
					To:    tok,
					Title: "Пропущен приём лекарства",
					Body:  fmt.Sprintf("%s не отметил «%s» в %s", m.patient, m.med, m.at.In(time.Local).Format("15:04")),
					Sound: "default",
					Data:  map[string]any{"type": "missed", "dose_id": m.id},
				})
			}
			err = n.push(ctx, msgs)
			if err == nil {
				sent += len(msgs)
			}
		}
		if err == nil {
			_, err = n.DB.Exec(ctx, `UPDATE doses SET relative_notified_at = $2 WHERE id = $1`, m.id, now)
		}
		if err != nil {
			log.Printf("приём %d: %v", m.id, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return sent, firstErr
}

func (n *Notifier) tokens(ctx context.Context, patientID int64) ([]string, error) {
	rows, err := n.DB.Query(ctx, `SELECT push_token FROM relatives WHERE patient_id = $1 AND push_token <> ''`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// push отправляет сообщения одним запросом. Не-2xx — ошибка; ошибки по отдельным
// токенам (Expo отвечает 200) только пишем в лог, повторять их бессмысленно.
func (n *Notifier) push(ctx context.Context, msgs []message) error {
	body, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.PushURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := n.Client.Do(req)
	if err != nil {
		return fmt.Errorf("отправка push: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("Expo ответил %d: %.200s", resp.StatusCode, data)
	}
	var res struct {
		Data []struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			Details struct {
				Error string `json:"error"`
			} `json:"details"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &res) == nil {
		for _, t := range res.Data {
			if t.Status == "error" {
				log.Printf("Expo не принял push: %s %s", t.Details.Error, t.Message)
			}
		}
	}
	return nil
}
