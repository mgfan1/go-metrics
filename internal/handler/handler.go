package handler

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/internal/storage"
)

type Pinger interface {
	PingContext(ctx context.Context) error
}

type MetricsHandler struct {
	store storage.Repository
	db    Pinger
	log   *zap.Logger
}

func New(store storage.Repository, db Pinger, log *zap.Logger) *MetricsHandler {
	return &MetricsHandler{store: store, db: db, log: log}
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
		if err := h.store.UpdateGauge(r.Context(), name, v); err != nil {
			h.storeFailed(w, err)
			return
		}
	case models.Counter:
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			http.Error(w, "counter должен быть целым", http.StatusBadRequest)
			return
		}
		if err := h.store.AddCounter(r.Context(), name, v); err != nil {
			h.storeFailed(w, err)
			return
		}
	default:
		http.Error(w, "неизвестный тип метрики", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
}

func (h *MetricsHandler) Value(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	switch chi.URLParam(r, "type") {
	case models.Gauge:
		v, err := h.store.Gauge(r.Context(), name)
		if err != nil {
			h.readFailed(w, r, err)
			return
		}
		_, _ = w.Write([]byte(strconv.FormatFloat(v, 'f', -1, 64)))
	case models.Counter:
		v, err := h.store.Counter(r.Context(), name)
		if err != nil {
			h.readFailed(w, r, err)
			return
		}
		_, _ = w.Write([]byte(strconv.FormatInt(v, 10)))
	default:
		http.Error(w, "неизвестный тип метрики", http.StatusBadRequest)
	}
}

func (h *MetricsHandler) storeFailed(w http.ResponseWriter, err error) {
	h.log.Warn("не сохранил метрику", zap.Error(err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (h *MetricsHandler) readFailed(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	h.log.Warn("не прочитал метрику", zap.Error(err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

type metricRow struct {
	Name  string
	Value string
}

var listPage = template.Must(template.New("list").Parse(
	`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Метрики</title></head>
<body>
<table>
{{range .}}<tr><td>{{.Name}}</td><td>{{.Value}}</td></tr>
{{end}}</table>
</body>
</html>`))

func (h *MetricsHandler) List(w http.ResponseWriter, r *http.Request) {
	gauges, counters, err := h.store.Snapshot(r.Context())
	if err != nil {
		h.log.Warn("не прочитал метрики", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	rows := make([]metricRow, 0, len(gauges)+len(counters))
	for name, v := range gauges {
		rows = append(rows, metricRow{name, strconv.FormatFloat(v, 'f', -1, 64)})
	}
	for name, v := range counters {
		rows = append(rows, metricRow{name, strconv.FormatInt(v, 10)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := listPage.Execute(w, rows); err != nil {
		h.log.Warn("не отрисовал страницу метрик", zap.Error(err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}
