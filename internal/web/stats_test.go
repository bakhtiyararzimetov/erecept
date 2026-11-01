package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erecept/internal/db/dbtest"
)

func TestPrescriptionStats(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	now := time.Now().Truncate(time.Second)
	for _, q := range []string{
		`INSERT INTO doctors (login, password_hash, full_name) VALUES ('d', 'x', 'Врач')`,
		`INSERT INTO patients (device_secret) VALUES ('patient-secret-0001')`,
		`INSERT INTO prescriptions (code, doctor_id, patient_id, patient_name, patient_birth, start_date, claimed_at)
			VALUES ('c1', 1, 1, 'Иванов', '1980-01-01', current_date, now())`,
		`INSERT INTO prescription_items (prescription_id, medicine_name, dosage, times_per_day, days, times)
			VALUES (1, 'Парацетамол', '1', 1, 3, '{08:00}'), (1, 'Аспирин', '1', 1, 3, '{08:00}')`,
	} {
		if _, err := pool.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	// Парацетамол: 3 отмечено, 1 пропущен, 1 впереди. Аспирин: 1 впереди (за 30 мин до now — ещё не пропуск).
	for i, c := range []struct {
		item  int
		ago   time.Duration
		taken bool
	}{
		{1, 10 * time.Hour, true}, {1, 9 * time.Hour, true}, {1, 8 * time.Hour, true},
		{1, 2 * time.Hour, false}, {1, -2 * time.Hour, false}, {2, 30 * time.Minute, false},
	} {
		var tk any
		if c.taken {
			tk = now
		}
		if _, err := pool.Exec(ctx, `INSERT INTO doses (prescription_id, item_id, scheduled_at, day_number, taken_at) VALUES (1, $1, $2, $3, $4)`,
			c.item, now.Add(-c.ago), i+1, tk); err != nil {
			t.Fatal(err)
		}
	}
	st, err := New(pool).prescriptionStats(ctx, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if st.Total != 6 || st.Taken != 3 || st.Missed != 1 || st.Upcoming != 2 || st.Percent != 75 {
		t.Errorf("итого: %+v", st)
	}
	if len(st.Items) != 2 || st.Items[0] != (ItemStats{"Парацетамол", 3, 1, 5}) || st.Items[1] != (ItemStats{"Аспирин", 0, 0, 1}) {
		t.Errorf("по лекарствам: %+v", st.Items)
	}
	empty, _ := New(pool).prescriptionStats(ctx, 999, now)
	if empty.Percent != 100 || empty.Total != 0 {
		t.Errorf("пустой: %+v", empty)
	}
}

func TestAdherencePageRenders(t *testing.T) {
	pool, h := newTestEnv(t)
	ck := doctorCookies(t, h, "doctor1")
	w := postForm(h, "/doctor/prescriptions", validForm(), ck)
	loc := w.Header().Get("Location")
	if loc == "" {
		t.Fatalf("нет перехода: %d", w.Code)
	}
	get := func(path string) string {
		req := httptest.NewRequest("GET", path, nil)
		for _, c := range ck {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d", path, rec.Code)
		}
		return rec.Body.String()
	}
	if b := get(loc); !strings.Contains(b, "Пациент ещё не отсканировал рецепт") {
		t.Error("до сканирования нет подсказки")
	}
	if _, err := pool.Exec(context.Background(), `UPDATE prescriptions SET start_date = current_date`); err != nil {
		t.Fatal(err)
	}
	b := get(loc)
	for _, want := range []string{"Выполнено", "Пропущено", "Впереди", `data-label="Всего"`, "100%"} {
		if !strings.Contains(b, want) {
			t.Errorf("страница без %q", want)
		}
	}
	if l := get("/doctor/prescriptions"); !strings.Contains(l, "100%") {
		t.Error("в списке нет процента")
	}
}
