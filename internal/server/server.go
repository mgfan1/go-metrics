// Package server запускает HTTP-сервер и штатно останавливает его по отмене контекста.
package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.uber.org/zap"
)

const shutdownTimeout = 5 * time.Second

// Server обслуживает HTTP-запросы до отмены контекста.
type Server struct {
	http *http.Server
	log  *zap.Logger
}

// New создаёт сервер на адресе addr с обработчиком handler.
func New(addr string, handler http.Handler, log *zap.Logger) *Server {
	return &Server{
		http: &http.Server{Addr: addr, Handler: handler},
		log:  log,
	}
}

// Run принимает запросы до отмены ctx, затем штатно останавливает сервер.
func (s *Server) Run(ctx context.Context) error {
	stopped := make(chan struct{})

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.http.Shutdown(shutdownCtx); err != nil {
			s.log.Warn("сервер не остановился штатно", zap.Error(err))
		}
		close(stopped)
	}()

	s.log.Info("сервер метрик запущен")

	if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	<-stopped

	return nil
}
