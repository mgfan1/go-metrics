package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

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
	cfg, err := parseFlags()
	if err != nil {
		return err
	}

	if err := logger.Initialize(); err != nil {
		return err
	}
	defer func() { _ = logger.Log.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store := storage.NewMemStorage()
	files := storage.NewFileStore(store, cfg.fileStorage)

	if cfg.restore {
		if err := files.Load(); err != nil {
			logger.Log.Info("не восстановил метрики", zap.Error(err))
		}
	}

	var repo storage.Repository = store
	if cfg.storeInterval <= 0 {
		repo = files.SyncRepository()
	} else {
		go files.RunPeriodic(ctx, time.Duration(cfg.storeInterval)*time.Second)
	}

	srv := &http.Server{
		Addr:    cfg.addr,
		Handler: handler.New(repo).Router(),
	}

	stopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Log.Info("сервер не остановился штатно", zap.Error(err))
		}
		close(stopped)
	}()

	logger.Log.Info("сервер метрик запущен",
		zap.String("addr", cfg.addr),
		zap.Int("store_interval", cfg.storeInterval),
		zap.String("file", cfg.fileStorage),
		zap.Bool("restore", cfg.restore),
	)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-stopped

	if err := files.Save(); err != nil {
		return err
	}
	logger.Log.Info("метрики сохранены", zap.String("file", cfg.fileStorage))

	return nil
}
