package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/internal/storage"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	h := New(storage.NewMemStorage(), nil, zap.NewNop())
	return httptest.NewServer(h.Router(zap.NewNop()))
}

func do(t *testing.T, ts *httptest.Server, method, path string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, ts.URL+path, nil)
	require.NoError(t, err)

	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestUpdateStatuses(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	cases := []struct {
		name string
		path string
		want int
	}{
		{"валидный gauge", "/update/gauge/Alloc/13.5", http.StatusOK},
		{"валидный counter", "/update/counter/PollCount/5", http.StatusOK},
		{"отрицательный gauge", "/update/gauge/Temp/-7.25", http.StatusOK},
		{"неизвестный тип", "/update/unknown/x/1", http.StatusBadRequest},
		{"gauge не число", "/update/gauge/Alloc/foo", http.StatusBadRequest},
		{"counter дробный", "/update/counter/PollCount/1.5", http.StatusBadRequest},
		{"нет значения", "/update/counter/PollCount", http.StatusNotFound},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _ := do(t, ts, http.MethodPost, c.path)
			assert.Equal(t, c.want, code)
		})
	}
}

func TestUpdateThenRead(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	post := func(path string) {
		code, _ := do(t, ts, http.MethodPost, path)
		require.Equal(t, http.StatusOK, code, path)
	}

	post("/update/gauge/Alloc/100.5")
	post("/update/gauge/Alloc/42.1")
	post("/update/counter/PollCount/5")
	post("/update/counter/PollCount/3")

	code, body := do(t, ts, http.MethodGet, "/value/gauge/Alloc")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "42.1", body)

	code, body = do(t, ts, http.MethodGet, "/value/counter/PollCount")
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "8", body)

	code, _ = do(t, ts, http.MethodGet, "/value/gauge/Unknown")
	assert.Equal(t, http.StatusNotFound, code)
}

type brokenStore struct {
	storage.Repository
	err error
}

func (s brokenStore) UpdateGauge(context.Context, string, float64) error { return s.err }

func (s brokenStore) AddCounter(context.Context, string, int64) error { return s.err }

func (s brokenStore) UpdateBatch(context.Context, []models.Metrics) error { return s.err }

func (s brokenStore) Gauge(context.Context, string) (float64, error) { return 0, s.err }

func (s brokenStore) Counter(context.Context, string) (int64, error) { return 0, s.err }

func (s brokenStore) Snapshot(context.Context) (map[string]float64, map[string]int64, error) {
	return nil, nil, s.err
}

func TestStorageErrorGivesServerError(t *testing.T) {
	store := brokenStore{Repository: storage.NewMemStorage(), err: errors.New("база упала")}
	ts := httptest.NewServer(New(store, nil, zap.NewNop()).Router(zap.NewNop()))
	defer ts.Close()

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"запись gauge", http.MethodPost, "/update/gauge/Alloc/1"},
		{"запись counter", http.MethodPost, "/update/counter/PollCount/1"},
		{"чтение gauge", http.MethodGet, "/value/gauge/Alloc"},
		{"чтение counter", http.MethodGet, "/value/counter/PollCount"},
		{"список метрик", http.MethodGet, "/"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _ := do(t, ts, c.method, c.path)
			assert.Equal(t, http.StatusInternalServerError, code)
		})
	}
}

func TestBatchStorageErrorGivesServerError(t *testing.T) {
	store := brokenStore{Repository: storage.NewMemStorage(), err: errors.New("база упала")}
	ts := httptest.NewServer(New(store, nil, zap.NewNop()).Router(zap.NewNop()))
	defer ts.Close()

	resp, _ := postJSON(t, ts, "/updates/", `[{"id":"Alloc","type":"gauge","value":1.5}]`)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestListPage(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	do(t, ts, http.MethodPost, "/update/gauge/Alloc/1")
	do(t, ts, http.MethodPost, "/update/counter/PollCount/7")

	code, body := do(t, ts, http.MethodGet, "/")
	assert.Equal(t, http.StatusOK, code)
	assert.True(t, strings.Contains(body, "Alloc"), "на странице нет Alloc")
	assert.True(t, strings.Contains(body, "PollCount"), "на странице нет PollCount")
}
