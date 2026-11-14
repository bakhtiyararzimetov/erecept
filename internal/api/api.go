// Package api: REST API (JSON) для мобильного приложения.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"erecept/internal/notifier"
)

type API struct {
	DB      *pgxpool.Pool
	PushURL string       // Expo Push API; пусто — боевой адрес
	Client  *http.Client // клиент для push; пусто — с таймаутом 15 с
}

var defaultPushClient = &http.Client{Timeout: 15 * time.Second}

func (a *API) pushURL() string {
	if a.PushURL != "" {
		return a.PushURL
	}
	return notifier.DefaultPushURL
}

func (a *API) pushClient() *http.Client {
	if a.Client != nil {
		return a.Client
	}
	return defaultPushClient
}

type ctxKey struct{}

var secretRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// Routes монтируется в main под /api.
func (a *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(cors)
	r.Get("/health", a.health)
	r.Get("/photos/{id}", a.getPhoto)
	r.Get("/medicines/search", a.searchMedicines)
	r.Get("/medicines/popular", a.popularMedicines)
	r.Get("/offers", a.offers)

	r.Group(func(r chi.Router) {
		r.Use(deviceSecret)
		r.Get("/prescriptions/{code}", a.getPrescription)
		r.Post("/prescriptions/{code}/claim", a.claim)
		r.Get("/me/prescriptions", a.myPrescriptions)
		r.Post("/doses/sync", a.syncDoses)
		r.Post("/sos", a.sos)
		r.Post("/push-token", a.pushToken)
		r.Post("/invites", a.createInvite)
		r.Post("/relatives/join", a.joinRelative)
		r.Get("/relative/overview", a.relativeOverview)
		r.Post("/items/{id}/photo", a.uploadPhoto)
	})
	return r
}

// cors разрешает запросы из Expo web; preflight отвечает 204.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, X-Device-Secret")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// deviceSecret проверяет заголовок X-Device-Secret и кладёт его в context.
func deviceSecret(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s := r.Header.Get("X-Device-Secret")
		if !secretRe.MatchString(s) {
			writeError(w, http.StatusUnauthorized, "нет X-Device-Secret")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, s)))
	})
}

func secretFrom(ctx context.Context) string {
	s, _ := ctx.Value(ctxKey{}).(string)
	return s
}

// patientID возвращает id пациента по секрету, создавая его при первом обращении.
func (a *API) patientID(ctx context.Context, secret string) (int64, error) {
	var id int64
	err := a.DB.QueryRow(ctx, `WITH ins AS (
			INSERT INTO patients (device_secret) VALUES ($1) ON CONFLICT (device_secret) DO NOTHING RETURNING id)
		SELECT id FROM ins UNION ALL SELECT id FROM patients WHERE device_secret = $1 LIMIT 1`, secret).Scan(&id)
	return id, err
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.DB.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "база недоступна")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func serverError(w http.ResponseWriter, what string, err error) {
	logf("%s: %v", what, err)
	writeError(w, http.StatusInternalServerError, "Ошибка сервера, попробуйте ещё раз")
}
