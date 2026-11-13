package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// appdist — веб-версия приложения пациента (Expo, собирается командой из README в mobile/).
// all: — чтобы попала папка _expo.
//
//go:embed all:appdist
var appFiles embed.FS

// patientApp — /app/*: файл сборки, а для маршрутов приложения (/app/scan, /app/explain…) — index.html.
func patientApp() http.Handler {
	dist, _ := fs.Sub(appFiles, "appdist")
	files := http.FileServerFS(dist)
	return http.StripPrefix("/app", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p != "" && p != "." {
			if st, err := fs.Stat(dist, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "_expo/") {
					rw.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // имена файлов с хешем
				}
				files.ServeHTTP(rw, r)
				return
			}
		}
		rw.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(rw, r, dist, "index.html")
	}))
}
