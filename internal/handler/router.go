package handler

import (
	"github.com/go-chi/chi/v5"

	"github.com/mgfan1/go-metrics/internal/middleware"
)

func (h *MetricsHandler) Router() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logging)
	r.Use(middleware.Gzip)

	r.Get("/", h.List)

	r.Route("/update", func(r chi.Router) {
		r.Post("/", h.UpdateJSON)
		r.Post("/{type}/{name}/{value}", h.Update)
	})

	r.Route("/value", func(r chi.Router) {
		r.Post("/", h.ValueJSON)
		r.Get("/{type}/{name}", h.Value)
	})

	return r
}
