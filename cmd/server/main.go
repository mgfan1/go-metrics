package main

import (
	"log"
	"net/http"

	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/handler"
	"github.com/mgfan1/go-metrics/internal/logger"
	"github.com/mgfan1/go-metrics/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg := parseFlags()

	if err := logger.Initialize(); err != nil {
		return err
	}
	defer func() { _ = logger.Log.Sync() }()

	store := storage.NewMemStorage()
	h := handler.New(store)

	logger.Log.Info("сервер метрик запущен", zap.String("addr", cfg.addr))

	return http.ListenAndServe(cfg.addr, h.Router())
}
