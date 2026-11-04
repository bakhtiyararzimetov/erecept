package web

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"erecept/internal/db/dbtest"
)

func liveServer(t *testing.T, key string) *httptest.Server {
	t.Helper()
	w := New(dbtest.New(t))
	w.PitchKey = key
	srv := httptest.NewServer(w.Routes())
	t.Cleanup(srv.Close)
	return srv
}

func setSlide(t *testing.T, base, body string) int {
	t.Helper()
	resp, err := http.Post(base+"/pitch/live", "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// Пульт: телефон с ключом листает, экран получает номер слайда по SSE.
func TestPitchLiveRemote(t *testing.T) {
	srv := liveServer(t, "secret")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/pitch/live", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type %q", ct)
	}
	lines := bufio.NewScanner(resp.Body)
	next := func() string {
		for lines.Scan() {
			if l := lines.Text(); strings.HasPrefix(l, "data: ") {
				return strings.TrimPrefix(l, "data: ")
			}
		}
		t.Fatal("поток оборвался")
		return ""
	}
	if got := next(); got != "1" {
		t.Fatalf("начальный слайд %q", got)
	}

	if code := setSlide(t, srv.URL, "key=wrong&slide=3"); code != http.StatusForbidden {
		t.Errorf("чужой ключ: код %d", code)
	}
	if code := setSlide(t, srv.URL, "key=secret&slide=0"); code != http.StatusBadRequest {
		t.Errorf("слайд 0: код %d", code)
	}
	if code := setSlide(t, srv.URL, "key=secret&slide=4"); code != http.StatusNoContent {
		t.Fatalf("код %d", code)
	}
	if got := next(); got != "4" {
		t.Fatalf("после переключения %q", got)
	}

	// Новый зритель сразу получает текущий слайд.
	req2, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/pitch/live", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	s2 := bufio.NewScanner(resp2.Body)
	for s2.Scan() {
		if l := s2.Text(); strings.HasPrefix(l, "data: ") {
			if l != "data: 4" {
				t.Errorf("новый зритель получил %q", l)
			}
			break
		}
	}
}

func TestPitchLiveOffWithoutKey(t *testing.T) {
	srv := liveServer(t, "")
	if code := setSlide(t, srv.URL, "key=&slide=2"); code != http.StatusNotFound {
		t.Errorf("без PITCH_KEY пульт выключен, код %d", code)
	}
}
