package handler

import "github.com/go-chi/chi/v5"

func (h *MetricsHandler) Router() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/update/{type}/{name}/{value}", h.Update)
	r.Get("/value/{type}/{name}", h.Value)

	return r
}
