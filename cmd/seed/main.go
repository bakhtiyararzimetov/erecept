// Команда seed: вручную загружает тестовые данные (врачи, аптеки, лекарства, цены).
package main

import (
	"context"
	"log"
	"os"

	"github.com/joho/godotenv"

	"erecept/internal/db"
	"erecept/internal/seed"
)

func main() {
	_ = godotenv.Load()
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
	if err := seed.Run(ctx, pool); err != nil {
		log.Fatal(err)
	}
	log.Println("тестовые данные загружены")
}
