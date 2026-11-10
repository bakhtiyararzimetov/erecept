package web

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"erecept/internal/boxcheck"
	"erecept/internal/i18n"
	"erecept/internal/models"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// BoxChecker сверяет фото упаковки с рецептом (boxcheck.Checker; в тестах — подделка).
type BoxChecker interface {
	Check(ctx context.Context, img []byte, items []boxcheck.Item, lang boxcheck.Lang) (boxcheck.Result, error)
}

const maxPhotoBytes = 6 << 20

// boxCheck — POST /r/{code}/check: пациент фотографирует купленную упаковку,
// ИИ отвечает, совпадает ли она с рецептом. Фото не сохраняется.
func (w *Web) boxCheck(rw http.ResponseWriter, r *http.Request) {
	noIndex(rw)
	if w.BoxCheck == nil {
		w.boxError(rw, r, http.StatusNotFound, "Проверка упаковки сейчас недоступна.")
		return
	}
	p, err := w.loadPrescriptionByCode(r.Context(), chi.URLParam(r, "code"))
	if errors.Is(err, pgx.ErrNoRows) {
		w.boxError(rw, r, http.StatusNotFound, "Рецепт не найден.")
		return
	}
	if err != nil {
		log.Printf("проверка упаковки: %v", err)
		w.boxError(rw, r, http.StatusInternalServerError, "Не удалось открыть рецепт, попробуйте ещё раз.")
		return
	}
	if !w.boxLimit.allow(clientIP(r), time.Now()) {
		w.boxError(rw, r, http.StatusTooManyRequests, "Слишком много проверок. Попробуйте позже.")
		return
	}
	r.Body = http.MaxBytesReader(rw, r.Body, maxPhotoBytes)
	f, _, err := r.FormFile("photo")
	if err != nil {
		w.boxError(rw, r, http.StatusBadRequest, "Сфотографируйте упаковку и отправьте фото ещё раз.")
		return
	}
	img, err := io.ReadAll(f)
	f.Close()
	if err != nil || len(img) == 0 {
		w.boxError(rw, r, http.StatusBadRequest, "Сфотографируйте упаковку и отправьте фото ещё раз.")
		return
	}
	items, err := w.boxItems(r.Context(), p.Items)
	if err != nil {
		log.Printf("проверка упаковки: %v", err)
		w.boxError(rw, r, http.StatusInternalServerError, "Не удалось открыть рецепт, попробуйте ещё раз.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	lang, footer := boxcheck.RU, boxcheck.Footer
	if i18n.FromContext(r.Context()) == i18n.EN {
		lang, footer = boxcheck.EN, boxcheck.FooterEN
	}
	res, err := w.BoxCheck.Check(ctx, img, items, lang)
	switch {
	case errors.Is(err, boxcheck.ErrImage):
		w.boxError(rw, r, http.StatusBadRequest, "Это не похоже на фото. Сфотографируйте упаковку ещё раз.")
		return
	case err != nil:
		log.Printf("проверка упаковки: %v", err)
		w.boxError(rw, r, http.StatusBadGateway, "ИИ сейчас не ответил. Попробуйте ещё раз через минуту.")
		return
	}
	kind := "warn"
	if res.Verdict == "match" || res.Verdict == "analog" {
		kind = "ok"
	}
	w.renderFragment(rw, r, http.StatusOK, "box_result", map[string]any{"Res": res, "Kind": kind, "Footer": footer})
}

// boxItems — строки рецепта с действующим веществом из справочника (если лекарство оттуда).
func (w *Web) boxItems(ctx context.Context, rx []models.PrescriptionItem) ([]boxcheck.Item, error) {
	var ids []int64
	for _, it := range rx {
		if it.MedicineID != nil {
			ids = append(ids, *it.MedicineID)
		}
	}
	type catalog struct{ substance, nameEN, substanceEN string }
	subst := map[int64]catalog{}
	if len(ids) > 0 {
		rows, err := w.DB.Query(ctx, `SELECT id, active_substance, name_en, substance_en FROM medicines WHERE id = ANY($1)`, ids)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var c catalog
			if err := rows.Scan(&id, &c.substance, &c.nameEN, &c.substanceEN); err != nil {
				return nil, err
			}
			subst[id] = c
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]boxcheck.Item, 0, len(rx))
	for _, it := range rx {
		bi := boxcheck.Item{Name: it.MedicineName, Dosage: it.Dosage}
		if it.MedicineID != nil {
			c := subst[*it.MedicineID]
			bi.Substance, bi.NameEN, bi.SubstanceEN = c.substance, c.nameEN, c.substanceEN
		}
		out = append(out, bi)
	}
	return out, nil
}

func (w *Web) boxError(rw http.ResponseWriter, r *http.Request, status int, text string) {
	w.renderFragment(rw, r, status, "box_result", map[string]any{"Error": tr(r, text), "Kind": "warn"})
}

// renderFragment выводит кусок страницы (ответ для fetch) с нужным кодом.
func (w *Web) renderFragment(rw http.ResponseWriter, r *http.Request, status int, page string, data map[string]any) {
	rw.Header().Set("Content-Type", "text/html; charset=utf-8")
	rw.WriteHeader(status)
	if err := w.mustTpl(r, page).ExecuteTemplate(rw, page+".html", data); err != nil {
		log.Printf("шаблон %s: %v", page, err)
	}
}

// clientIP: Render стоит за Cloudflare — он кладёт адрес клиента в CF-Connecting-IP
// (или True-Client-IP). Иначе последний адрес X-Forwarded-For: первые клиент может подставить сам.
func clientIP(r *http.Request) string {
	for _, h := range []string{"CF-Connecting-IP", "True-Client-IP"} {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			return v
		}
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// rateLimit — лимит проверок с одного адреса и общий, чтобы не потратить баланс ИИ.
type rateLimit struct {
	mu               sync.Mutex
	perIP, global    int
	ipWin, globalWin time.Duration
	byIP             map[string][]time.Time
	all              []time.Time
}

func newRateLimit(perIP int, ipWin time.Duration, global int, globalWin time.Duration) *rateLimit {
	return &rateLimit{perIP: perIP, ipWin: ipWin, global: global, globalWin: globalWin, byIP: map[string][]time.Time{}}
}

func (l *rateLimit) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.all = recent(l.all, now.Add(-l.globalWin))
	mine := recent(l.byIP[ip], now.Add(-l.ipWin))
	if len(l.all) >= l.global || len(mine) >= l.perIP {
		if len(mine) == 0 {
			delete(l.byIP, ip)
		} else {
			l.byIP[ip] = mine
		}
		return false
	}
	l.all = append(l.all, now)
	l.byIP[ip] = append(mine, now)
	return true
}

// recent оставляет отметки новее since.
func recent(ts []time.Time, since time.Time) []time.Time {
	i := 0
	for i < len(ts) && !ts[i].After(since) {
		i++
	}
	return ts[i:]
}
