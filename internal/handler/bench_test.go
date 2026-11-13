package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/hash"
	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/internal/storage"
)

const benchKey = "ключ для бенчмарка"

var agentGauges = []string{
	"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys", "HeapAlloc", "HeapIdle",
	"HeapInuse", "HeapObjects", "HeapReleased", "HeapSys", "LastGC", "Lookups", "MCacheInuse",
	"MCacheSys", "MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC", "NumGC",
	"OtherSys", "PauseTotalNs", "StackInuse", "StackSys", "Sys", "TotalAlloc", "RandomValue",
	"TotalMemory", "FreeMemory", "CPUutilization1", "CPUutilization2", "CPUutilization3", "CPUutilization4",
}

func agentBatch(tb testing.TB) []models.Metrics {
	tb.Helper()

	delta := int64(5)
	batch := []models.Metrics{{ID: "PollCount", MType: models.Counter, Delta: &delta}}
	for i, name := range agentGauges {
		value := float64(i+1) * 1234.5678
		batch = append(batch, models.Metrics{ID: name, MType: models.Gauge, Value: &value})
	}
	return batch
}

func serveOK(b *testing.B, router http.Handler, req *http.Request) {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		b.Fatalf("%s %s: сервер ответил %d", req.Method, req.URL.Path, rec.Code)
	}
}

func BenchmarkUpdatesFromAgent(b *testing.B) {
	router := New(storage.NewMemStorage(), nil, nil, zap.NewNop()).Router(zap.NewNop(), benchKey)

	body, err := json.Marshal(agentBatch(b))
	require.NoError(b, err)
	compressed := gzipBody(b, string(body)).Bytes()
	signature := hash.Sign(body, benchKey)

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(compressed))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set(hash.Header, signature)
		serveOK(b, router, req)
	}
}

func BenchmarkUpdateJSON(b *testing.B) {
	router := New(storage.NewMemStorage(), nil, nil, zap.NewNop()).Router(zap.NewNop(), "")
	body := []byte(`{"id":"PollCount","type":"counter","delta":1}`)

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/update/", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		serveOK(b, router, req)
	}
}

func BenchmarkValue(b *testing.B) {
	store := storage.NewMemStorage()
	require.NoError(b, store.UpdateBatch(b.Context(), agentBatch(b)))
	router := New(store, nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	for b.Loop() {
		serveOK(b, router, httptest.NewRequest(http.MethodGet, "/value/gauge/HeapAlloc", nil))
	}
}

func BenchmarkListPage(b *testing.B) {
	store := storage.NewMemStorage()
	require.NoError(b, store.UpdateBatch(b.Context(), agentBatch(b)))
	router := New(store, nil, nil, zap.NewNop()).Router(zap.NewNop(), "")

	for b.Loop() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		serveOK(b, router, req)
	}
}
