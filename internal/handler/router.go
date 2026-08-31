package handler

import (
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/middleware"
)

func (h *MetricsHandler) Router(log *zap.Logger, key string) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logging(log))
	r.Use(middleware.Gzip)
	r.Use(middleware.Hash(key, log))

	r.Get("/", h.List)
	r.Get("/ping", h.Ping)

	r.Route("/update", func(r chi.Router) {
		r.Post("/", h.UpdateJSON)
		r.Post("/{type}/{name}/{value}", h.Update)
	})

	r.Route("/updates", func(r chi.Router) {
		r.Post("/", h.UpdatesJSON)
	})

	r.Route("/value", func(r chi.Router) {
		r.Post("/", h.ValueJSON)
		r.Get("/{type}/{name}", h.Value)
	})

	return r
}
