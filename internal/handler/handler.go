package handler

import (
	"html/template"
	"net/http"
	"sort"
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

func (h *MetricsHandler) Value(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	switch chi.URLParam(r, "type") {
	case models.Gauge:
		v, ok := h.store.Gauge(name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(strconv.FormatFloat(v, 'f', -1, 64)))
	case models.Counter:
		v, ok := h.store.Counter(name)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(strconv.FormatInt(v, 10)))
	default:
		http.Error(w, "неизвестный тип метрики", http.StatusBadRequest)
	}
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

func (h *MetricsHandler) List(w http.ResponseWriter, _ *http.Request) {
	gauges, counters := h.store.Snapshot()

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
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
