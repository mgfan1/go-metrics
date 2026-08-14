package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/handler"
	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/internal/storage"
)

type captured struct {
	path    string
	headers http.Header
	raw     []byte
}

func newCollector() (*httptest.Server, func() []captured) {
	var mu sync.Mutex
	var got []captured

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)

		mu.Lock()
		got = append(got, captured{path: r.URL.Path, headers: r.Header.Clone(), raw: raw})
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))

	return srv, func() []captured {
		mu.Lock()
		defer mu.Unlock()
		return append([]captured(nil), got...)
	}
}

func unpack(t *testing.T, raw []byte) []models.Metrics {
	t.Helper()

	zr, err := gzip.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)

	body, err := io.ReadAll(zr)
	require.NoError(t, err, "gzip-поток оборван")
	require.NoError(t, zr.Close())

	var batch []models.Metrics
	require.NoError(t, json.Unmarshal(body, &batch))
	return batch
}

func TestPoll(t *testing.T) {
	a := New("localhost:8080", time.Second, time.Second, zap.NewNop())
	a.poll()
	a.poll()

	if a.pollCount != 2 {
		t.Errorf("pollCount = %d, хотел 2", a.pollCount)
	}
	if _, ok := a.gauges["Alloc"]; !ok {
		t.Error("после poll нет метрики Alloc")
	}
	if len(a.gauges) != 28 {
		t.Errorf("собрал %d gauge, хотел 28", len(a.gauges))
	}
}

func TestReport(t *testing.T) {
	srv, dump := newCollector()
	defer srv.Close()

	a := New(strings.TrimPrefix(srv.URL, "http://"), time.Second, time.Second, zap.NewNop())
	a.poll()
	a.report()

	got := dump()
	require.Len(t, got, 1, "метрики должны уходить одним запросом")
	assert.Equal(t, "/updates/", got[0].path)

	batch := unpack(t, got[0].raw)
	require.Len(t, batch, 29)

	names := make(map[string]models.Metrics, len(batch))
	for _, m := range batch {
		names[m.ID] = m
	}

	alloc, ok := names["Alloc"]
	require.True(t, ok, "не отправлен gauge Alloc")
	assert.Equal(t, models.Gauge, alloc.MType)
	assert.NotNil(t, alloc.Value)
	assert.Nil(t, alloc.Delta)

	poll, ok := names["PollCount"]
	require.True(t, ok, "не отправлен counter PollCount")
	assert.Equal(t, models.Counter, poll.MType)
	require.NotNil(t, poll.Delta)
	assert.Equal(t, int64(1), *poll.Delta)
	assert.Nil(t, poll.Value)
}

func TestReportSendsGzip(t *testing.T) {
	srv, dump := newCollector()
	defer srv.Close()

	a := New(strings.TrimPrefix(srv.URL, "http://"), time.Second, time.Second, zap.NewNop())
	a.poll()
	a.report()

	got := dump()
	require.Len(t, got, 1)

	c := got[0]
	assert.Equal(t, "application/json", c.headers.Get("Content-Type"))
	assert.Equal(t, "gzip", c.headers.Get("Content-Encoding"))

	require.GreaterOrEqual(t, len(c.raw), 2)
	assert.Equal(t, []byte{0x1f, 0x8b}, c.raw[:2], "тело не сжато gzip")

	for _, m := range unpack(t, c.raw) {
		assert.NotEmpty(t, m.ID)
	}
}

func TestSendMetricPayload(t *testing.T) {
	value := 42.5
	delta := int64(7)

	cases := []struct {
		name    string
		metric  models.Metrics
		present string
		absent  string
	}{
		{"gauge", models.Metrics{ID: "Alloc", MType: models.Gauge, Value: &value}, "value", "delta"},
		{"counter", models.Metrics{ID: "PollCount", MType: models.Counter, Delta: &delta}, "delta", "value"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, dump := newCollector()
			defer srv.Close()

			a := New(strings.TrimPrefix(srv.URL, "http://"), time.Second, time.Second, zap.NewNop())
			require.NoError(t, a.send([]models.Metrics{c.metric}))

			got := dump()
			require.Len(t, got, 1)

			zr, err := gzip.NewReader(bytes.NewReader(got[0].raw))
			require.NoError(t, err)
			body, err := io.ReadAll(zr)
			require.NoError(t, err)

			var fields []map[string]any
			require.NoError(t, json.Unmarshal(body, &fields))
			require.Len(t, fields, 1)

			assert.Contains(t, fields[0], c.present)
			assert.NotContains(t, fields[0], c.absent)
		})
	}
}

func TestReportResetsPollCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := New(strings.TrimPrefix(srv.URL, "http://"), time.Second, time.Second, zap.NewNop())
	a.poll()
	a.poll()
	a.report()

	if a.pollCount != 0 {
		t.Errorf("после report pollCount = %d, хотел 0", a.pollCount)
	}
}

func TestReportKeepsPollCountOnFailure(t *testing.T) {
	srv, _ := newCollector()
	addr := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()

	a := New(addr, time.Second, time.Second, zap.NewNop())
	a.poll()
	a.poll()
	a.report()

	assert.Equal(t, int64(2), a.pollCount, "при недоступном сервере счётчик опросов терять нельзя")
}

func TestSendReturnsErrorOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	value := 1.0
	a := New(strings.TrimPrefix(srv.URL, "http://"), time.Second, time.Second, zap.NewNop())

	err := a.send([]models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &value}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestAgentSendsToRealServer(t *testing.T) {
	store := storage.NewMemStorage()
	srv := httptest.NewServer(handler.New(store, nil, zap.NewNop()).Router(zap.NewNop()))
	defer srv.Close()

	a := New(strings.TrimPrefix(srv.URL, "http://"), time.Second, time.Second, zap.NewNop())
	a.poll()
	a.report()

	ctx := context.Background()

	v, err := store.Gauge(ctx, "Alloc")
	require.NoError(t, err, "сервер не сохранил Alloc")
	assert.Positive(t, v)

	d, err := store.Counter(ctx, "PollCount")
	require.NoError(t, err, "сервер не сохранил PollCount")
	assert.Equal(t, int64(1), d)
}
