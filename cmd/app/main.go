package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
	_ "time/tzdata" // часовые пояса внутри бинарника, чтобы Asia/Almaty работал в любом контейнере

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"erecept/internal/api"
	"erecept/internal/db"
	"erecept/internal/seed"
	"erecept/internal/web"
)

func main() {
	// .env нужен для запуска без Docker. В docker-compose переменные приходят сами.
	_ = godotenv.Load()

	tz := env("TZ", "Asia/Almaty")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		log.Fatalf("неизвестный часовой пояс %q: %v", tz, err)
	}
	time.Local = loc

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("не задан DATABASE_URL (см. .env.example)")
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatal(err)
	}

	autoSeed(ctx, pool)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	a := &api.API{DB: pool}
	r.Mount("/api", a.Routes())
	r.Mount("/", web.New(pool).Routes())

	go keepAwake()

	addr := ":" + env("PORT", "8080")
	log.Printf("сервер запущен на %s (часовой пояс %s)", addr, tz)
	log.Fatal(http.ListenAndServe(addr, r))
}

// autoSeed загружает демо-данные в пустую базу (отключается SEED_DEMO=0).
func autoSeed(ctx context.Context, pool *pgxpool.Pool) {
	if os.Getenv("SEED_DEMO") == "0" {
		return
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM doctors`).Scan(&n); err != nil {
		log.Printf("проверка тестовых данных: %v", err)
		return
	}
	if n > 0 {
		return
	}
	if err := seed.Run(ctx, pool); err != nil {
		log.Printf("не удалось загрузить тестовые данные: %v", err)
		return
	}
	log.Println("загружены тестовые данные")
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
