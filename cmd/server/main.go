package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/audit"
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

	storeLog := logger.With(zap.String("component", "storage"))

	var db *sql.DB
	var pinger handler.Pinger

	if cfg.DatabaseDSN != "" {
		opened, err := sql.Open("pgx", cfg.DatabaseDSN)
		if err != nil {
			logger.Warn("не открыл соединение с базой", zap.Error(err))
		} else {
			opened.SetMaxOpenConns(10)
			opened.SetMaxIdleConns(10)
			opened.SetConnMaxIdleTime(4 * time.Minute)

			db = opened
			pinger = opened
		}
	}
	defer func() {
		if db != nil {
			db.Close()
		}
	}()

	var repo storage.Repository
	var files *storage.FileStore

	if db != nil {
		pg, err := storage.NewPGStorage(ctx, db, storeLog)
		if err != nil {
			logger.Warn("не подготовил хранилище в базе", zap.Error(err))
		} else {
			repo = pg
			logger.Info("метрики хранятся в базе данных")
		}
	}

	if repo == nil {
		store := storage.NewMemStorage()
		files, err = storage.NewFileStore(store, cfg.FileStorage, cfg.Restore, storeLog)
		if err != nil {
			logger.Warn("не восстановил метрики", zap.Error(err))
		}

		repo = store
		if cfg.StoreInterval <= 0 {
			repo = files.SyncRepository()
		} else {
			go files.RunPeriodic(ctx, time.Duration(cfg.StoreInterval)*time.Second)
		}
	}

	var auditor handler.Auditor
	if cfg.AuditFile != "" || cfg.AuditURL != "" {
		pub, err := newAudit(cfg, logger.With(zap.String("component", "audit")))
		if err != nil {
			return err
		}
		defer func() {
			if err := pub.Close(); err != nil {
				logger.Warn("не закрыл аудит", zap.Error(err))
			}
		}()

		auditor = pub
		logger.Info("аудит включён", zap.String("file", cfg.AuditFile), zap.String("url", cfg.AuditURL))
	}

	metrics := handler.New(repo, pinger, auditor, logger.With(zap.String("component", "handler")))
	router := metrics.Router(logger.With(zap.String("component", "middleware")), cfg.Key)
	srv := server.New(cfg.Addr, router, logger.With(zap.String("component", "server")))

	if err := srv.Run(ctx); err != nil {
		return err
	}

	if files != nil {
		if err := files.Close(); err != nil {
			return err
		}
		logger.Info("метрики сохранены")
	}

	return nil
}

func newAudit(cfg config.Server, log *zap.Logger) (*audit.Publisher, error) {
	var observers []audit.Observer

	if cfg.AuditURL != "" {
		remote, err := audit.NewHTTPObserver(cfg.AuditURL)
		if err != nil {
			return nil, err
		}
		observers = append(observers, remote)
	}
	if cfg.AuditFile != "" {
		file, err := audit.NewFileObserver(cfg.AuditFile)
		if err != nil {
			return nil, err
		}
		observers = append(observers, file)
	}

	pub := audit.NewPublisher(log)
	for _, o := range observers {
		pub.Register(o)
	}

	return pub, nil
}
