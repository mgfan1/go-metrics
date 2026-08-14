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
		if err := h.store.UpdateGauge(r.Context(), m.ID, *m.Value); err != nil {
			h.storeFailed(w, err)
			return
		}
		m.Delta = nil
	case models.Counter:
		if m.Delta == nil {
			http.Error(w, "не задано значение counter", http.StatusBadRequest)
			return
		}
		if err := h.store.AddCounter(r.Context(), m.ID, *m.Delta); err != nil {
			h.storeFailed(w, err)
			return
		}
		total, err := h.store.Counter(r.Context(), m.ID)
		if err != nil {
			h.readFailed(w, r, err)
			return
		}
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
		v, err := h.store.Gauge(r.Context(), m.ID)
		if err != nil {
			h.readFailed(w, r, err)
			return
		}
		m.Value = &v
		m.Delta = nil
	case models.Counter:
		v, err := h.store.Counter(r.Context(), m.ID)
		if err != nil {
			h.readFailed(w, r, err)
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
