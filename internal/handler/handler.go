package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/internal/storage"
)

type MetricsHandler struct {
	store storage.Repository
}

func New(store storage.Repository) *MetricsHandler {
	return &MetricsHandler{store: store}
}

func (h *MetricsHandler) Update(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		http.Error(w, "не задано имя метрики", http.StatusNotFound)
		return
	}

	raw := chi.URLParam(r, "value")
	switch chi.URLParam(r, "type") {
	case models.Gauge:
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			http.Error(w, "gauge должен быть числом", http.StatusBadRequest)
			return
		}
		h.store.UpdateGauge(name, v)
	case models.Counter:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "counter должен быть целым", http.StatusBadRequest)
			return
		}
		h.store.AddCounter(name, v)
	default:
		http.Error(w, "неизвестный тип метрики", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}
