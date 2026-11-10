// Package i18n: язык интерфейса (ru по умолчанию, en) и перевод строк.
// Ключ перевода — сама русская строка: шаблон пишет {{t "Рецепт"}}, а en.go даёт английский вариант.
// Нет перевода — показывается русский текст.
package i18n

import (
	"context"
	"fmt"
	"net/http"
)

type Lang string

const (
	RU Lang = "ru"
	EN Lang = "en"
)

// CookieName — cookie с выбранным языком; ставится по ссылке с ?lang=en|ru.
const CookieName = "lang"

type ctxKey struct{}

func parse(s string) (Lang, bool) {
	switch s {
	case "ru":
		return RU, true
	case "en":
		return EN, true
	}
	return RU, false
}

// Middleware: ?lang=en|ru выбирает язык и запоминает его в cookie на год; иначе язык из cookie.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := RU
		if q, ok := parse(r.URL.Query().Get("lang")); ok {
			l = q
			http.SetCookie(w, &http.Cookie{Name: CookieName, Value: string(l), Path: "/", MaxAge: 365 * 24 * 3600, SameSite: http.SameSiteLaxMode})
		} else if c, err := r.Cookie(CookieName); err == nil {
			l, _ = parse(c.Value)
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, l)))
	})
}

// FromContext — язык запроса (ru, если middleware не стоял).
func FromContext(ctx context.Context) Lang {
	if l, ok := ctx.Value(ctxKey{}).(Lang); ok {
		return l
	}
	return RU
}

// T переводит русскую строку на язык l.
func T(l Lang, ru string) string {
	if l == EN {
		if s, ok := en[ru]; ok {
			return s
		}
	}
	return ru
}

// Tf — T для строки формата.
func Tf(l Lang, ru string, args ...any) string {
	return fmt.Sprintf(T(l, ru), args...)
}

// Has сообщает, есть ли английский перевод (для теста шаблонов).
func Has(ru string) bool {
	_, ok := en[ru]
	return ok
}

// T и Tf как методы — для функций шаблона, привязанных к языку.
func (l Lang) T(ru string) string               { return T(l, ru) }
func (l Lang) Tf(ru string, args ...any) string { return Tf(l, ru, args...) }
