package handler

import "github.com/go-chi/chi/v5"

func (h *MetricsHandler) Router() chi.Router {
	r := chi.NewRouter()

	r.Post("/update/{type}/{name}/{value}", h.Update)

	return r
}
