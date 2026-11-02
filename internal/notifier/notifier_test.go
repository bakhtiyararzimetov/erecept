package notifier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"erecept/internal/db/dbtest"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeExpo struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	status int
}

func newFakeExpo(t *testing.T) *fakeExpo {
	f := &fakeExpo{status: 200}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.bodies = append(f.bodies, string(b))
		st := f.status
		f.mu.Unlock()
		w.WriteHeader(st)
		w.Write([]byte(`{"data":[{"status":"error","details":{"error":"DeviceNotRegistered"}}]}`))
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeExpo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bodies)
}

// setup очищает таблицы и создаёт рецепт с одним лекарством, пациентом и родственником с токеном.
// claimedAgo — сколько назад рецепт привязан.
func setup(t *testing.T, pool *pgxpool.Pool, now time.Time, claimedAgo time.Duration) (itemID int64) {
	t.Helper()
	ctx := context.Background()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`TRUNCATE doctors, patients RESTART IDENTITY CASCADE`)
	mustExec(`INSERT INTO doctors (login, password_hash, full_name) VALUES ('d', 'x', 'Врач')`)
	mustExec(`INSERT INTO patients (device_secret) VALUES ('patient-secret-0001')`)
	mustExec(`INSERT INTO relatives (patient_id, device_secret, name, push_token) VALUES
		(1, 'relative-secret-01', 'Айгуль', 'ExponentPushToken[abc]'),
		(1, 'relative-secret-02', 'Без токена', '')`)
	mustExec(`INSERT INTO prescriptions (code, doctor_id, patient_id, patient_name, patient_birth, start_date, claimed_at)
		VALUES ('c1', 1, 1, 'Иванов Иван Иванович', '1980-01-01', current_date, $1)`, now.Add(-claimedAgo))
	if err := pool.QueryRow(ctx, `INSERT INTO prescription_items (prescription_id, medicine_name, dosage, times_per_day, days, times)
		VALUES (1, 'Парацетамол', '1 таблетка', 1, 3, '{08:00}') RETURNING id`).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	return itemID
}

func addDose(t *testing.T, pool *pgxpool.Pool, item int64, at time.Time, taken bool) {
	t.Helper()
	var tk any
	if taken {
		tk = at
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO doses (prescription_id, item_id, scheduled_at, day_number, taken_at)
		VALUES (1, $1, $2, 1, $3)`, item, at, tk); err != nil {
		t.Fatal(err)
	}
}

func notified(t *testing.T, pool *pgxpool.Pool) bool {
	t.Helper()
	var ok bool
	if err := pool.QueryRow(context.Background(), `SELECT relative_notified_at IS NOT NULL FROM doses LIMIT 1`).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

func TestCheckOnce(t *testing.T) {
	pool := dbtest.New(t)
	now := time.Now().Truncate(time.Second)
	run := func(t *testing.T, f *fakeExpo) (int, error) {
		n := New(pool)
		n.PushURL = f.URL
		n.Now = func() time.Time { return now }
		return n.CheckOnce(context.Background())
	}

	t.Run("Sends", func(t *testing.T) {
		f := newFakeExpo(t)
		item := setup(t, pool, now, 24*time.Hour)
		at := now.Add(-70 * time.Minute)
		addDose(t, pool, item, at, false)
		sent, err := run(t, f)
		if err != nil || sent != 1 || f.count() != 1 {
			t.Fatalf("sent=%d err=%v запросов=%d", sent, err, f.count())
		}
		var msgs []map[string]any
		if err := json.Unmarshal([]byte(f.bodies[0]), &msgs); err != nil || len(msgs) != 1 {
			t.Fatalf("тело %q: %v", f.bodies[0], err)
		}
		body, _ := msgs[0]["body"].(string)
		want := "Иванов Иван Иванович не отметил(а) «Парацетамол» в " + at.In(time.Local).Format("15:04")
		if body != want || msgs[0]["to"] != "ExponentPushToken[abc]" || msgs[0]["title"] != "Пропущен приём лекарства" {
			t.Errorf("сообщение: %v", msgs[0])
		}
		if !notified(t, pool) {
			t.Error("relative_notified_at не выставлен")
		}
		sent, err = run(t, f)
		if err != nil || sent != 0 || f.count() != 1 {
			t.Errorf("повтор: sent=%d err=%v запросов=%d", sent, err, f.count())
		}
	})
	t.Run("SkipsRecent", func(t *testing.T) {
		f := newFakeExpo(t)
		item := setup(t, pool, now, 24*time.Hour)
		addDose(t, pool, item, now.Add(-30*time.Minute), false)
		if sent, _ := run(t, f); sent != 0 || f.count() != 0 {
			t.Errorf("sent=%d запросов=%d", sent, f.count())
		}
	})
	t.Run("SkipsOld", func(t *testing.T) {
		f := newFakeExpo(t)
		item := setup(t, pool, now, 24*time.Hour)
		addDose(t, pool, item, now.Add(-4*time.Hour), false)
		if sent, _ := run(t, f); sent != 0 || f.count() != 0 {
			t.Errorf("sent=%d запросов=%d", sent, f.count())
		}
	})
	t.Run("SkipsBeforeClaim", func(t *testing.T) {
		f := newFakeExpo(t)
		item := setup(t, pool, now, 30*time.Minute) // привязан 30 мин назад
		addDose(t, pool, item, now.Add(-70*time.Minute), false)
		if sent, _ := run(t, f); sent != 0 || f.count() != 0 {
			t.Errorf("sent=%d запросов=%d", sent, f.count())
		}
	})
	t.Run("SkipsTaken", func(t *testing.T) {
		f := newFakeExpo(t)
		item := setup(t, pool, now, 24*time.Hour)
		addDose(t, pool, item, now.Add(-70*time.Minute), true)
		if sent, _ := run(t, f); sent != 0 || f.count() != 0 {
			t.Errorf("sent=%d запросов=%d", sent, f.count())
		}
	})
	t.Run("NoRelativesStillMarked", func(t *testing.T) {
		f := newFakeExpo(t)
		item := setup(t, pool, now, 24*time.Hour)
		pool.Exec(context.Background(), `UPDATE relatives SET push_token = ''`)
		addDose(t, pool, item, now.Add(-70*time.Minute), false)
		sent, err := run(t, f)
		if err != nil || sent != 0 || f.count() != 0 || !notified(t, pool) {
			t.Errorf("sent=%d err=%v запросов=%d notified=%v", sent, err, f.count(), notified(t, pool))
		}
	})
	t.Run("PushError", func(t *testing.T) {
		f := newFakeExpo(t)
		f.status = 500
		item := setup(t, pool, now, 24*time.Hour)
		addDose(t, pool, item, now.Add(-70*time.Minute), false)
		sent, err := run(t, f)
		if sent != 0 || err == nil {
			t.Errorf("sent=%d err=%v", sent, err)
		}
		if f.count() != 1 || notified(t, pool) {
			t.Errorf("запросов=%d notified=%v, ждали 1 и false", f.count(), notified(t, pool))
		}
	})
}
