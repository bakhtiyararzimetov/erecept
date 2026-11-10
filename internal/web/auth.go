package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName   = "erecept_session"
	sessionTTL   = 7 * 24 * time.Hour
	roleDoctor   = "doctor"
	rolePharmacy = "pharmacy"
)

// SessionUser — вошедший врач или аптека.
type SessionUser struct {
	Role string // "doctor" | "pharmacy"
	ID   int64
	Name string
}

type ctxKey struct{}

func currentUser(r *http.Request) *SessionUser {
	u, _ := r.Context().Value(ctxKey{}).(*SessionUser)
	return u
}

// loadUser по cookie находит сессию и кладёт пользователя в контекст.
func (w *Web) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
			if u := w.userByToken(r.Context(), c.Value); u != nil {
				r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
			}
		}
		next.ServeHTTP(rw, r)
	})
}

func (w *Web) userByToken(ctx context.Context, token string) *SessionUser {
	var role string
	var id int64
	err := w.DB.QueryRow(ctx, `SELECT role, user_id FROM web_sessions WHERE token = $1 AND expires_at > now()`, token).Scan(&role, &id)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("поиск сессии: %v", err)
		}
		return nil
	}
	q := `SELECT full_name FROM doctors WHERE id = $1`
	if role == rolePharmacy {
		q = `SELECT name FROM pharmacies WHERE id = $1`
	}
	u := &SessionUser{Role: role, ID: id}
	if err := w.DB.QueryRow(ctx, q, id).Scan(&u.Name); err != nil {
		log.Printf("поиск пользователя сессии: %v", err)
		return nil
	}
	return u
}

// requireRole пускает только вошедших с нужной ролью: гостя ведёт на /login,
// чужую роль — на её собственную страницу.
func requireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			u := currentUser(r)
			switch {
			case u == nil:
				http.Redirect(rw, r, "/login", http.StatusSeeOther)
			case u.Role != role:
				http.Redirect(rw, r, homePath(u.Role), http.StatusSeeOther)
			default:
				next.ServeHTTP(rw, r)
			}
		})
	}
}

func homePath(role string) string {
	if role == roleDoctor {
		return "/doctor"
	}
	return "/pharmacy"
}

func (w *Web) home(rw http.ResponseWriter, r *http.Request) {
	if u := currentUser(r); u != nil {
		http.Redirect(rw, r, homePath(u.Role), http.StatusSeeOther)
		return
	}
	http.Redirect(rw, r, "/login", http.StatusSeeOther)
}

func (w *Web) loginForm(rw http.ResponseWriter, r *http.Request) {
	if u := currentUser(r); u != nil {
		http.Redirect(rw, r, homePath(u.Role), http.StatusSeeOther)
		return
	}
	w.render(rw, r, "login", nil)
}

func (w *Web) loginSubmit(rw http.ResponseWriter, r *http.Request) {
	login := strings.TrimSpace(r.FormValue("login"))
	password := r.FormValue("password")

	role, id, ok := w.checkCredentials(r.Context(), login, password)
	if !ok {
		w.render(rw, r, "login", map[string]any{"Error": tr(r, "Неверный логин или пароль"), "Login": login})
		return
	}

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Printf("генерация токена: %v", err)
		http.Error(rw, tr(r, "Не удалось войти, попробуйте ещё раз"), http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(b)
	expires := time.Now().Add(sessionTTL)
	_, err := w.DB.Exec(r.Context(), `INSERT INTO web_sessions (token, role, user_id, expires_at) VALUES ($1, $2, $3, $4)`,
		token, role, id, expires)
	if err != nil {
		log.Printf("создание сессии: %v", err)
		http.Error(rw, tr(r, "Не удалось войти, попробуйте ещё раз"), http.StatusInternalServerError)
		return
	}
	http.SetCookie(rw, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
	http.Redirect(rw, r, homePath(role), http.StatusSeeOther)
}

// checkCredentials ищет логин сначала среди врачей, потом среди аптек.
func (w *Web) checkCredentials(ctx context.Context, login, password string) (role string, id int64, ok bool) {
	if login == "" || password == "" {
		return "", 0, false
	}
	for _, t := range []struct{ role, table string }{{roleDoctor, "doctors"}, {rolePharmacy, "pharmacies"}} {
		var hash string
		err := w.DB.QueryRow(ctx, `SELECT id, password_hash FROM `+t.table+` WHERE login = $1`, login).Scan(&id, &hash)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				log.Printf("поиск логина: %v", err)
			}
			continue
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
			return t.role, id, true
		}
		return "", 0, false
	}
	return "", 0, false
}

func (w *Web) logout(rw http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		if _, err := w.DB.Exec(r.Context(), `DELETE FROM web_sessions WHERE token = $1`, c.Value); err != nil {
			log.Printf("удаление сессии: %v", err)
		}
	}
	http.SetCookie(rw, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(rw, r, "/login", http.StatusSeeOther)
}
