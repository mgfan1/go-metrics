package handler

import (
	"github.com/go-chi/chi/v5"

	"github.com/mgfan1/go-metrics/internal/middleware"
)

func (h *MetricsHandler) Router() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logging)

	r.Get("/", h.List)
	r.Post("/update/{type}/{name}/{value}", h.Update)
	r.Get("/value/{type}/{name}", h.Value)

	return r
}
