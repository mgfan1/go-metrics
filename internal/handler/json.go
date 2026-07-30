package handler

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
)

func (h *MetricsHandler) UpdateJSON(w http.ResponseWriter, r *http.Request) {
	var m models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}
	if m.ID == "" {
		http.Error(w, "не задано имя метрики", http.StatusNotFound)
		return
	}

	switch m.MType {
	case models.Gauge:
		if m.Value == nil {
			http.Error(w, "не задано значение gauge", http.StatusBadRequest)
			return
		}
		h.store.UpdateGauge(m.ID, *m.Value)
		m.Delta = nil
	case models.Counter:
		if m.Delta == nil {
			http.Error(w, "не задано значение counter", http.StatusBadRequest)
			return
		}
		h.store.AddCounter(m.ID, *m.Delta)
		total, _ := h.store.Counter(m.ID)
		m.Delta = &total
		m.Value = nil
	default:
		http.Error(w, "неизвестный тип метрики", http.StatusBadRequest)
		return
	}

	h.writeMetric(w, m)
}

func (h *MetricsHandler) ValueJSON(w http.ResponseWriter, r *http.Request) {
	var m models.Metrics
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		http.Error(w, "некорректный JSON", http.StatusBadRequest)
		return
	}

	switch m.MType {
	case models.Gauge:
		v, ok := h.store.Gauge(m.ID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		m.Value = &v
		m.Delta = nil
	case models.Counter:
		v, ok := h.store.Counter(m.ID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		m.Delta = &v
		m.Value = nil
	default:
		http.Error(w, "неизвестный тип метрики", http.StatusBadRequest)
		return
	}

	h.writeMetric(w, m)
}

func (h *MetricsHandler) writeMetric(w http.ResponseWriter, m models.Metrics) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(m); err != nil {
		h.log.Warn("не отправил ответ", zap.Error(err))
	}
}
