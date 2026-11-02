package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"erecept/internal/db/dbtest"
)

func init() {
	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		panic(err)
	}
	time.Local = loc
}

type env struct {
	t    *testing.T
	pool *pgxpool.Pool
	h    http.Handler
}

func (e *env) do(method, path, secret string, body any) *httptest.ResponseRecorder {
	var rd *bytes.Reader
	if s, ok := body.(string); ok {
		rd = bytes.NewReader([]byte(s))
	} else if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	if secret != "" {
		req.Header.Set("X-Device-Secret", secret)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("не JSON: %s", w.Body.String())
	}
	return v
}

// newPrescription: рецепт с 1 лекарством, 2 раза в день, 3 дня. Возвращает code и id позиции.
func (e *env) newPrescription(code string) int64 {
	return e.newPrescriptionClaimedAt(code, 0)
}

// newPrescriptionClaimedAt закрепляет claimed_at на сегодня в hour:00 по местному времени,
// чтобы число приёмов в первый день не зависело от часа запуска теста (0 — полночь, все приёмы остаются).
func (e *env) newPrescriptionClaimedAt(code string, hour int) int64 {
	ctx := context.Background()
	var docID, rxID, itemID int64
	err := e.pool.QueryRow(ctx, `INSERT INTO doctors (login, password_hash, full_name, clinic)
		VALUES ($1, 'x', 'Ахметова Динара Сериковна', 'Поликлиника №5')
		ON CONFLICT (login) DO UPDATE SET login = EXCLUDED.login RETURNING id`, "doc").Scan(&docID)
	if err != nil {
		e.t.Fatal(err)
	}
	err = e.pool.QueryRow(ctx, `INSERT INTO prescriptions (code, doctor_id, patient_name, patient_birth)
		VALUES ($1, $2, 'Иванов Иван Иванович', '1985-03-12') RETURNING id`, code, docID).Scan(&rxID)
	if err != nil {
		e.t.Fatal(err)
	}
	err = e.pool.QueryRow(ctx, `INSERT INTO prescription_items (prescription_id, medicine_name, dosage, times_per_day, days, meal, times)
		VALUES ($1, 'Парацетамол', '1 таблетка', 2, 3, 'after', '{08:00,20:00}') RETURNING id`, rxID).Scan(&itemID)
	if err != nil {
		e.t.Fatal(err)
	}
	now := time.Now().In(time.Local)
	at := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.Local)
	if _, err := e.pool.Exec(ctx, `UPDATE prescriptions SET claimed_at = $1 WHERE id = $2`, at, rxID); err != nil {
		e.t.Fatal(err)
	}
	return itemID
}

type claimResp struct {
	Prescription PrescriptionDTO `json:"prescription"`
	Doses        []DoseDTO       `json:"doses"`
}

func (e *env) claim(code, secret string) claimResp {
	w := e.do("POST", "/prescriptions/"+code+"/claim", secret, nil)
	if w.Code != 200 {
		e.t.Fatalf("claim: %d %s", w.Code, w.Body.String())
	}
	return decode[claimResp](e.t, w)
}

func TestAPI(t *testing.T) {
	pool := dbtest.New(t)
	e := &env{t: t, pool: pool, h: (&API{DB: pool}).Routes()}

	t.Run("NoSecret401", func(t *testing.T) {
		e.t = t
		w := e.do("GET", "/me/prescriptions", "", nil)
		if w.Code != 401 || !strings.Contains(w.Body.String(), "нет X-Device-Secret") {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if w := e.do("GET", "/me/prescriptions", "short", nil); w.Code != 401 {
			t.Fatalf("короткий секрет: %d", w.Code)
		}
		// preflight и CORS
		w = e.do("OPTIONS", "/me/prescriptions", "", nil)
		if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Fatalf("preflight: %d %v", w.Code, w.Header())
		}
	})

	t.Run("ClaimCreatesDoses", func(t *testing.T) {
		e.t = t
		e.newPrescription("code-create")
		w := e.do("GET", "/prescriptions/code-create", "secret-create-0000001", nil)
		if w.Code != 200 {
			t.Fatalf("get: %d", w.Code)
		}
		if e.do("GET", "/prescriptions/nope", "secret-create-0000001", nil).Code != 404 {
			t.Fatal("ожидался 404")
		}
		r := e.claim("code-create", "secret-create-0000001")
		if len(r.Doses) != 6 {
			t.Fatalf("doses = %d", len(r.Doses))
		}
		today := time.Now().In(time.Local).Format("2006-01-02")
		if r.Prescription.StartDate == nil || *r.Prescription.StartDate != today {
			t.Fatalf("start_date = %v", r.Prescription.StartDate)
		}
		if r.Prescription.DoctorName != "Ахметова Динара Сериковна" || r.Prescription.PatientBirth != "1985-03-12" {
			t.Fatalf("dto: %+v", r.Prescription)
		}
		it := r.Prescription.Items[0]
		if it.MealLabel != "после еды" || len(it.Times) != 2 {
			t.Fatalf("item: %+v", it)
		}
		for i, d := range r.Doses {
			if d.DayNumber != i/2+1 || d.TotalDays != 3 {
				t.Fatalf("dose %d: %+v", i, d)
			}
		}
		if !strings.HasSuffix(r.Doses[0].ScheduledAt, "08:00:00+05:00") {
			t.Fatalf("scheduled_at = %s", r.Doses[0].ScheduledAt)
		}
		my := decode[[]claimResp](t, e.do("GET", "/me/prescriptions", "secret-create-0000001", nil))
		if len(my) != 1 || len(my[0].Doses) != 6 {
			t.Fatalf("me: %+v", my)
		}
	})

	t.Run("ClaimSkipsPastDosesOfDayOne", func(t *testing.T) {
		e.t = t
		e.newPrescriptionClaimedAt("code-late", 13)
		r := e.claim("code-late", "secret-late-00000001")
		if len(r.Doses) != 5 {
			t.Fatalf("doses = %d, ждали 5", len(r.Doses))
		}
		if !strings.Contains(r.Doses[0].ScheduledAt, "T20:00:00") || r.Doses[0].DayNumber != 1 || r.Doses[0].TotalDays != 3 {
			t.Fatalf("первый приём: %+v", r.Doses[0])
		}
		for i, d := range r.Doses[1:] {
			if d.DayNumber != i/2+2 || d.TotalDays != 3 {
				t.Fatalf("dose %d: %+v", i+1, d)
			}
		}
		if again := e.claim("code-late", "secret-late-00000001"); len(again.Doses) != 5 {
			t.Fatalf("после второго claim %d", len(again.Doses))
		}
	})

	t.Run("ClaimTwiceNoDuplicates", func(t *testing.T) {
		e.t = t
		e.newPrescription("code-twice")
		a := e.claim("code-twice", "secret-twice-a-000001")
		b := e.claim("code-twice", "secret-twice-a-000001")
		if len(b.Doses) != 6 {
			t.Fatalf("после второго claim %d", len(b.Doses))
		}
		for i := range a.Doses {
			if a.Doses[i].ID != b.Doses[i].ID {
				t.Fatal("id приёмов изменились")
			}
		}
		e.do("POST", "/doses/sync", "secret-twice-a-000001", map[string]any{
			"marks": []map[string]any{{"dose_id": a.Doses[0].ID, "taken_at": "2026-11-01T08:05:00+05:00"}},
		})
		c := e.claim("code-twice", "secret-twice-b-000001")
		if len(c.Doses) != 6 || c.Doses[0].ID != a.Doses[0].ID {
			t.Fatalf("другой телефон: %+v", c.Doses)
		}
		if c.Doses[0].TakenAt == nil {
			t.Fatal("отметка потерялась")
		}
		var n int
		pool.QueryRow(context.Background(), `SELECT count(*) FROM doses d JOIN prescriptions p ON p.id=d.prescription_id WHERE p.code='code-twice'`).Scan(&n)
		if n != 6 {
			t.Fatalf("в базе %d приёмов", n)
		}
		if my := decode[[]claimResp](t, e.do("GET", "/me/prescriptions", "secret-twice-a-000001", nil)); len(my) != 0 {
			t.Fatal("рецепт остался у старого телефона")
		}
	})

	t.Run("SyncIdempotent", func(t *testing.T) {
		e.t = t
		e.newPrescription("code-sync")
		r := e.claim("code-sync", "secret-sync-a-0000001")
		other := e.claim2("code-sync-other", "secret-sync-b-0000001")
		marks := map[string]any{"marks": []map[string]any{
			{"dose_id": r.Doses[0].ID, "taken_at": "2026-11-01T08:05:00+05:00"},
			{"dose_id": r.Doses[1].ID, "taken_at": "2026-11-01T20:05:00+05:00"},
			{"dose_id": other, "taken_at": "2026-11-01T20:05:00+05:00"},
			{"dose_id": 999999, "taken_at": "2026-11-01T20:05:00+05:00"},
			{"dose_id": r.Doses[2].ID},
			{"dose_id": r.Doses[3].ID, "taken_at": "2999-01-01T00:00:00+05:00"},
		}}
		got := decode[map[string]int](t, e.do("POST", "/doses/sync", "secret-sync-a-0000001", marks))
		if got["updated"] != 3 {
			t.Fatalf("updated = %d", got["updated"])
		}
		var zero, future *time.Time
		pool.QueryRow(context.Background(), `SELECT taken_at FROM doses WHERE id=$1`, r.Doses[2].ID).Scan(&zero)
		pool.QueryRow(context.Background(), `SELECT taken_at FROM doses WHERE id=$1`, r.Doses[3].ID).Scan(&future)
		if zero != nil {
			t.Fatal("отметка без taken_at не должна сохраняться")
		}
		if future == nil || future.After(time.Now().Add(time.Minute)) {
			t.Fatalf("будущее taken_at не обрезано: %v", future)
		}
		marks2 := map[string]any{"marks": []map[string]any{
			{"dose_id": r.Doses[0].ID, "taken_at": "2026-11-02T10:00:00+05:00"},
		}}
		got = decode[map[string]int](t, e.do("POST", "/doses/sync", "secret-sync-a-0000001", marks2))
		if got["updated"] != 0 {
			t.Fatalf("повтор updated = %d", got["updated"])
		}
		my := decode[[]claimResp](t, e.do("GET", "/me/prescriptions", "secret-sync-a-0000001", nil))
		if my[0].Doses[0].TakenAt == nil || *my[0].Doses[0].TakenAt != "2026-11-01T08:05:00+05:00" {
			t.Fatalf("taken_at = %v", my[0].Doses[0].TakenAt)
		}
		var taken *time.Time
		pool.QueryRow(context.Background(), `SELECT taken_at FROM doses WHERE id=$1`, other).Scan(&taken)
		if taken != nil {
			t.Fatal("чужой приём отмечен")
		}
		if e.do("POST", "/doses/sync", "secret-sync-a-0000001", "{bad").Code != 400 {
			t.Fatal("ожидался 400")
		}
	})

	t.Run("InviteJoinOverview", func(t *testing.T) {
		e.t = t
		e.newPrescription("code-inv")
		p := "secret-inv-patient-01"
		r := e.claim("code-inv", p)
		// один приём отмечен
		e.do("POST", "/doses/sync", p, map[string]any{"marks": []map[string]any{{"dose_id": r.Doses[0].ID, "taken_at": time.Now().Format(time.RFC3339)}}})
		inv := decode[map[string]string](t, e.do("POST", "/invites", p, nil))
		code := inv["code"]
		if len(code) != 6 || inv["expires_at"] == "" {
			t.Fatalf("invite: %v", inv)
		}
		rel := "secret-inv-relative-1"
		w := e.do("POST", "/relatives/join", rel, map[string]string{"code": code, "name": "Айгуль"})
		if w.Code != 200 || decode[map[string]string](t, w)["patient_name"] != "Иванов Иван Иванович" {
			t.Fatalf("join: %d %s", w.Code, w.Body.String())
		}
		w = e.do("POST", "/relatives/join", "secret-inv-relative-2", map[string]string{"code": code, "name": "Берик"})
		if w.Code != 400 || !strings.Contains(w.Body.String(), "Код неверный или истёк") {
			t.Fatalf("повтор: %d %s", w.Code, w.Body.String())
		}
		type ov struct {
			Patients []struct {
				PatientName  string `json:"patient_name"`
				Taken        int    `json:"taken"`
				Missed       int    `json:"missed"`
				Upcoming     int    `json:"upcoming"`
				RecentMissed []any  `json:"recent_missed"`
			} `json:"patients"`
		}
		o := decode[ov](t, e.do("GET", "/relative/overview", rel, nil))
		if len(o.Patients) != 1 || o.Patients[0].PatientName != "Иванов Иван Иванович" || o.Patients[0].Taken != 1 ||
			o.Patients[0].Taken+o.Patients[0].Missed+o.Patients[0].Upcoming != 6 || o.Patients[0].RecentMissed == nil {
			t.Fatalf("overview: %s", e.do("GET", "/relative/overview", rel, nil).Body.String())
		}
		po := e.do("GET", "/relative/overview", p, nil)
		if !strings.Contains(po.Body.String(), `"patients":[]`) {
			t.Fatalf("пациент: %s", po.Body.String())
		}
	})

	t.Run("PushTokenStored", func(t *testing.T) {
		e.t = t
		s := "secret-push-0000000001"
		e.do("GET", "/me/prescriptions", s, nil) // создаёт пациента
		w := e.do("POST", "/push-token", s, map[string]string{"token": "ExponentPushToken[abc]"})
		if w.Code != 200 {
			t.Fatalf("%d", w.Code)
		}
		var tok string
		pool.QueryRow(context.Background(), `SELECT push_token FROM patients WHERE device_secret=$1`, s).Scan(&tok)
		if tok != "ExponentPushToken[abc]" {
			t.Fatalf("token = %q", tok)
		}
	})

	t.Run("PhotoUploadOwnerOnly", func(t *testing.T) {
		e.t = t
		item := e.newPrescription("code-photo")
		owner := "secret-photo-owner-001"
		e.claim("code-photo", owner)
		png := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{1, 2, 3}, 50)...)
		upload := func(secret string, data []byte) *httptest.ResponseRecorder {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("photo", "p.png")
			fw.Write(data)
			mw.Close()
			req := httptest.NewRequest("POST", "/items/"+itoa(item)+"/photo", &buf)
			req.Header.Set("Content-Type", mw.FormDataContentType())
			req.Header.Set("X-Device-Secret", secret)
			w := httptest.NewRecorder()
			e.h.ServeHTTP(w, req)
			return w
		}
		w := upload(owner, png)
		if w.Code != 200 {
			t.Fatalf("upload: %d %s", w.Code, w.Body.String())
		}
		url := decode[map[string]string](t, w)["photo_url"]
		if url != "/api/photos/"+itoa(item) {
			t.Fatalf("url = %s", url)
		}
		g := e.do("GET", "/photos/"+itoa(item), "", nil)
		if g.Code != 200 || !bytes.Equal(g.Body.Bytes(), png) || g.Header().Get("Content-Type") != "image/png" {
			t.Fatalf("get photo: %d %s", g.Code, g.Header().Get("Content-Type"))
		}
		if w := upload("secret-photo-stranger1", png); w.Code != 404 {
			t.Fatalf("чужой: %d", w.Code)
		}
		if w := upload(owner, []byte("GIF89a not an image")); w.Code != 400 {
			t.Fatalf("не картинка: %d", w.Code)
		}
	})
}

// claim2 создаёт отдельный рецепт, привязывает к секрету и возвращает id первого приёма.
func (e *env) claim2(code, secret string) int64 {
	e.newPrescription(code)
	r := e.claim(code, secret)
	return r.Doses[0].ID
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestPrescriptionDTOIncludesExplanation(t *testing.T) {
	pool := dbtest.New(t)
	e := &env{t: t, pool: pool, h: (&API{DB: pool}).Routes()}
	ctx := context.Background()
	itemID := e.newPrescription("explain1")
	var withID, withoutID int64
	if err := pool.QueryRow(ctx, `INSERT INTO medicines (name, active_substance) VALUES ('Парацетамол', 'парацетамол') RETURNING id`).Scan(&withID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO medicines (name, active_substance) VALUES ('Нурофен', 'ибупрофен') RETURNING id`).Scan(&withoutID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE prescription_items SET medicine_id = $1 WHERE id = $2`, withID, itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO prescription_items (prescription_id, position, medicine_name, medicine_id, dosage, times_per_day, days, meal, times)
		SELECT prescription_id, 1, 'Нурофен', $1, '1 таблетка', 1, 3, 'after', '{09:00}' FROM prescription_items WHERE id = $2`, withoutID, itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO medicine_explanations (medicine_id, data) VALUES ($1, $2)`, withID,
		`{"purpose":"Снижает температуру.","high_risk":false,"source":{"reg_number":"РК-ЛС-5№1"}}`); err != nil {
		t.Fatal(err)
	}

	w := e.do("POST", "/prescriptions/explain1/claim", "secret-explain-0123456789", nil)
	if w.Code != 200 {
		t.Fatalf("claim: %d %s", w.Code, w.Body.String())
	}
	var raw struct {
		Prescription struct {
			Items []map[string]json.RawMessage `json:"items"`
		} `json:"prescription"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	items := raw.Prescription.Items
	if len(items) != 2 {
		t.Fatalf("позиций %d", len(items))
	}
	var ex struct {
		Purpose string `json:"purpose"`
		Source  struct {
			RegNumber string `json:"reg_number"`
		} `json:"source"`
	}
	if err := json.Unmarshal(items[0]["explanation"], &ex); err != nil || ex.Purpose != "Снижает температуру." || ex.Source.RegNumber != "РК-ЛС-5№1" {
		t.Fatalf("памятка у Парацетамола: %s", items[0]["explanation"])
	}
	if v, ok := items[1]["explanation"]; !ok || string(v) != "null" {
		t.Fatalf("у Нурофена ожидался explanation: null, получено %q (есть поле: %v)", v, ok)
	}
}
