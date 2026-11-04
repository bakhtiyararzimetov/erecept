package web

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// pitchLive — пульт для презентации /pitch: телефон с ключом переключает слайд,
// все открытые экраны получают номер по SSE. Состояние в памяти: после перезапуска — слайд 1.
type pitchLive struct {
	mu    sync.Mutex
	slide int
	subs  map[chan int]struct{}
}

func newPitchLive() *pitchLive {
	return &pitchLive{slide: 1, subs: map[chan int]struct{}{}}
}

func (p *pitchLive) set(n int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.slide = n
	for ch := range p.subs {
		select {
		case ch <- n:
		default: // зритель не успевает — получит следующий номер
		}
	}
}

func (p *pitchLive) subscribe() (chan int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ch := make(chan int, 4)
	p.subs[ch] = struct{}{}
	return ch, p.slide
}

func (p *pitchLive) unsubscribe(ch chan int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.subs, ch)
}

// pitchLiveStream — GET /pitch/live: поток «data: N» при каждом переключении.
func (w *Web) pitchLiveStream(rw http.ResponseWriter, r *http.Request) {
	if w.PitchKey == "" {
		http.NotFound(rw, r)
		return
	}
	rc := http.NewResponseController(rw)
	rw.Header().Set("Content-Type", "text/event-stream")
	rw.Header().Set("Cache-Control", "no-cache")
	rw.Header().Set("X-Accel-Buffering", "no")
	ch, cur := w.live.subscribe()
	defer w.live.unsubscribe(ch)
	send := func(n int) bool {
		if _, err := fmt.Fprintf(rw, "data: %d\n\n", n); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send(cur) {
		return
	}
	ping := time.NewTicker(20 * time.Second) // прокси Render не рвёт тихое соединение
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case n := <-ch:
			if !send(n) {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(rw, ": ping\n\n"); err != nil || rc.Flush() != nil {
				return
			}
		}
	}
}

// pitchLiveSet — POST /pitch/live (key, slide): переключение с телефона.
func (w *Web) pitchLiveSet(rw http.ResponseWriter, r *http.Request) {
	if w.PitchKey == "" {
		http.NotFound(rw, r)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.PostFormValue("key")), []byte(w.PitchKey)) != 1 {
		http.Error(rw, "неверный ключ пульта", http.StatusForbidden)
		return
	}
	n, err := strconv.Atoi(r.PostFormValue("slide"))
	if err != nil || n < 1 || n > 100 {
		http.Error(rw, "номер слайда от 1 до 100", http.StatusBadRequest)
		return
	}
	w.live.set(n)
	rw.WriteHeader(http.StatusNoContent)
}
