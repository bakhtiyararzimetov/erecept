package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"erecept/internal/db/dbtest"
	"erecept/internal/notifier"
)

type fakeExpo struct {
	mu     sync.Mutex
	msgs   []notifier.Message
	status int
	srv    *httptest.Server
}

func newFakeExpo(t *testing.T) *fakeExpo {
	f := &fakeExpo{status: 200}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.status == 200 {
			var m []notifier.Message
			json.Unmarshal(b, &m)
			f.msgs = append(f.msgs, m...)
		}
		w.WriteHeader(f.status)
		w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeExpo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.msgs)
}

func sosEnv(t *testing.T) (*env, *fakeExpo) {
	pool := dbtest.New(t)
	f := newFakeExpo(t)
	e := &env{t: t, pool: pool, h: (&API{DB: pool, PushURL: f.srv.URL}).Routes()}
	return e, f
}

// addRelative привязывает родственника с push-токеном к пациенту с секретом secret.
func (e *env) addRelative(secret, relSecret, token string) {
	e.t.Helper()
	pid, err := (&API{DB: e.pool}).patientID(e.t.Context(), secret)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.pool.Exec(e.t.Context(), `INSERT INTO relatives (patient_id, device_secret, name, push_token)
		VALUES ($1, $2, 'Родня', $3)`, pid, relSecret, token); err != nil {
		e.t.Fatal(err)
	}
}

const sosSecret = "secret-sos-patient-01"

func TestSOSNotifiesRelatives(t *testing.T) {
	e, f := sosEnv(t)
	e.newPrescription("code-sos")
	e.claim("code-sos", sosSecret)
	e.addRelative(sosSecret, "secret-sos-rel-0000001", "ExponentPushToken[a]")
	e.addRelative(sosSecret, "secret-sos-rel-0000002", "ExponentPushToken[b]")

	w := e.do("POST", "/sos", sosSecret, nil)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"notified":2}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if f.count() != 2 {
		t.Fatalf("push = %d", f.count())
	}
	m := f.msgs[0]
	if m.Title != "Иванов Иван Иванович нажал(а) «Мне плохо»" || m.Body != "Позвоните и узнайте, всё ли в порядке" || m.Data["type"] != "sos" {
		t.Fatalf("сообщение: %+v", m)
	}
}

func TestSOSThrottle(t *testing.T) {
	e, f := sosEnv(t)
	e.addRelative(sosSecret, "secret-sos-rel-0000001", "ExponentPushToken[a]")
	if w := e.do("POST", "/sos", sosSecret, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"notified":1`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	w := e.do("POST", "/sos", sosSecret, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"throttled":true`) || !strings.Contains(w.Body.String(), `"notified":0`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if f.count() != 1 {
		t.Fatalf("push = %d", f.count())
	}
	// спустя 3 минуты можно снова
	if _, err := e.pool.Exec(t.Context(), `UPDATE patients SET last_sos_at = now() - interval '3 minutes'`); err != nil {
		t.Fatal(err)
	}
	if w := e.do("POST", "/sos", sosSecret, nil); !strings.Contains(w.Body.String(), `"notified":1`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}

func TestSOSNoRelatives(t *testing.T) {
	e, f := sosEnv(t)
	w := e.do("POST", "/sos", sosSecret, nil)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"notified":0}` {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if f.count() != 0 {
		t.Fatal("push без родственников")
	}
	if e.do("POST", "/sos", "", nil).Code != 401 {
		t.Fatal("ожидался 401")
	}
}

func TestSOSPushFailureResetsThrottle(t *testing.T) {
	e, f := sosEnv(t)
	e.addRelative(sosSecret, "secret-sos-rel-0000001", "ExponentPushToken[a]")
	f.status = 500
	w := e.do("POST", "/sos", sosSecret, nil)
	if w.Code != 502 || !strings.Contains(w.Body.String(), "Не удалось отправить уведомление родственникам") {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	f.mu.Lock()
	f.status = 200
	f.mu.Unlock()
	if w := e.do("POST", "/sos", sosSecret, nil); w.Code != 200 || !strings.Contains(w.Body.String(), `"notified":1`) {
		t.Fatalf("повтор: %d %s", w.Code, w.Body.String())
	}
}

// Приложение ушло в фон во время звонка 103 и оборвало запрос — push всё равно доходит.
func TestSOSPushSurvivesClientCancel(t *testing.T) {
	pool := dbtest.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var got atomic.Int32
	expo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel() // клиент оборвал запрос, пока push в пути
		time.Sleep(100 * time.Millisecond)
		got.Add(1)
		w.Write([]byte(`{"data":[]}`))
	}))
	t.Cleanup(expo.Close)
	e := &env{t: t, pool: pool, h: (&API{DB: pool, PushURL: expo.URL}).Routes()}
	e.newPrescription("code-sos-cancel")
	e.claim("code-sos-cancel", sosSecret)
	e.addRelative(sosSecret, "secret-sos-rel-0000009", "ExponentPushToken[c]")

	req := httptest.NewRequest("POST", "/sos", nil).WithContext(ctx)
	req.Header.Set("X-Device-Secret", sosSecret)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	if w.Code != 200 || got.Load() != 1 {
		t.Fatalf("push должен уйти несмотря на отмену: %d %s", w.Code, w.Body.String())
	}
}
