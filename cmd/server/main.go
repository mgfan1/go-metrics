package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/config"
	"github.com/mgfan1/go-metrics/internal/handler"
	"github.com/mgfan1/go-metrics/internal/server"
	"github.com/mgfan1/go-metrics/internal/storage"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	code := 0
	if err := run(logger); err != nil {
		logger.Error("сервер остановлен с ошибкой", zap.Error(err))
		code = 1
	}

	_ = logger.Sync()
	os.Exit(code)
}

func run(logger *zap.Logger) error {
	cfg, err := config.ParseServer()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := storage.NewMemStorage()
	files, err := storage.NewFileStore(store, cfg.FileStorage, cfg.Restore,
		logger.With(zap.String("component", "storage")))
	if err != nil {
		logger.Warn("не восстановил метрики", zap.Error(err))
	}

	var repo storage.Repository = store
	if cfg.StoreInterval <= 0 {
		repo = files.SyncRepository()
	} else {
		go files.RunPeriodic(ctx, time.Duration(cfg.StoreInterval)*time.Second)
	}

	metrics := handler.New(repo, logger.With(zap.String("component", "handler")))
	router := metrics.Router(logger.With(zap.String("component", "middleware")))
	srv := server.New(cfg.Addr, router, logger.With(zap.String("component", "server")))

	if err := srv.Run(ctx); err != nil {
		return err
	}

	if err := files.Close(); err != nil {
		return err
	}
	logger.Info("метрики сохранены")

	return nil
}
