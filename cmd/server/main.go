package main

import (
	"log"
	"net/http"
	"os"

	"clocksync/internal/api"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	staticDir := os.Getenv("STATIC_DIR")
	if staticDir == "" {
		staticDir = "web/dist"
	}
	log.Printf("clocksync 服务启动：http://localhost%s（静态目录 %s）", addr, staticDir)
	log.Fatal(http.ListenAndServe(addr, api.NewHandler(staticDir)))
}
