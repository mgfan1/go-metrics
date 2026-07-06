package main

import (
	"log"
	"net/http"

	"github.com/mgfan1/go-metrics/internal/handler"
	"github.com/mgfan1/go-metrics/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := parseFlags()

	store := storage.NewMemStorage()
	h := handler.New(store)

	log.Printf("сервер метрик слушает %s", cfg.addr)
	return http.ListenAndServe(cfg.addr, h.Router())
}
