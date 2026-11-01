package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

// keepAwake не даёт бесплатному Render усыпить сервер: раз в 10 минут заходит
// на свой публичный адрес. Render сам задаёт RENDER_EXTERNAL_URL; локально его нет,
// и функция ничего не делает.
//
// Пингуем только в часы KEEP_AWAKE_HOURS (по умолчанию «8-23» по местному времени),
// потому что 750 бесплатных часов в месяц общие на все сервисы аккаунта.
func keepAwake() {
	base := os.Getenv("RENDER_EXTERNAL_URL")
	if base == "" {
		return
	}
	var from, to int
	if _, err := fmt.Sscanf(env("KEEP_AWAKE_HOURS", "8-23"), "%d-%d", &from, &to); err != nil {
		log.Printf("KEEP_AWAKE_HOURS: ожидается вид «8-23», самопинг выключен: %v", err)
		return
	}
	log.Printf("самопинг %s/api/health с %d:00 до %d:00", base, from, to)

	client := &http.Client{Timeout: 30 * time.Second}
	for range time.Tick(10 * time.Minute) {
		if h := time.Now().Hour(); h < from || h >= to {
			continue
		}
		resp, err := client.Get(base + "/api/health")
		if err != nil {
			log.Println("самопинг:", err)
			continue
		}
		resp.Body.Close()
	}
}
