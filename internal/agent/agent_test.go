package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/config"
	"github.com/mgfan1/go-metrics/internal/handler"
	"github.com/mgfan1/go-metrics/internal/hash"
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

func newAgent(serverURL, key string) *Agent {
	return New(config.Agent{
		Addr:           strings.TrimPrefix(serverURL, "http://"),
		ReportInterval: 1,
		PollInterval:   1,
		Key:            key,
		RateLimit:      1,
	}, zap.NewNop())
}

func report(t *testing.T, a *Agent) {
	t.Helper()
	a.deliver(t.Context(), a.snapshot(), a.log)
}

func gaugeReportJob() reportJob {
	value := 1.0
	return reportJob{batch: []models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &value}}}
}

func batchOf(n int) reportJob {
	delta := int64(5)

	batch := make([]models.Metrics, 0, n)
	batch = append(batch, models.Metrics{ID: "PollCount", MType: models.Counter, Delta: &delta})
	for i := 1; i < n; i++ {
		value := float64(i)
		batch = append(batch, models.Metrics{ID: "Gauge" + strconv.Itoa(i), MType: models.Gauge, Value: &value})
	}

	return reportJob{batch: batch, delta: delta}
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
	a := newAgent("localhost:8080", "")
	a.poll()
	a.poll()

	if a.pollCount.Load() != 2 {
		t.Errorf("pollCount = %d, хотел 2", a.pollCount.Load())
	}
	if _, ok := a.gauges["Alloc"]; !ok {
		t.Error("после poll нет метрики Alloc")
	}
	if len(a.gauges) != 28 {
		t.Errorf("собрал %d gauge, хотел 28", len(a.gauges))
	}
}

func TestPollSystem(t *testing.T) {
	a := newAgent("localhost:8080", "")
	a.pollSystem()

	a.mu.RLock()
	defer a.mu.RUnlock()

	require.Contains(t, a.gauges, "TotalMemory")
	assert.Positive(t, a.gauges["TotalMemory"])
	assert.Contains(t, a.gauges, "FreeMemory")
	assert.Contains(t, a.gauges, "CPUutilization1", "нет утилизации первого ядра")

	cores, err := cpu.Counts(true)
	require.NoError(t, err)
	assert.Contains(t, a.gauges, "CPUutilization"+strconv.Itoa(cores), "метрики не по числу ядер")
}

func TestSnapshotIncludesSystemGauges(t *testing.T) {
	a := newAgent("localhost:8080", "")
	a.poll()
	a.pollSystem()

	names := make(map[string]models.Metrics)
	for _, m := range a.snapshot().batch {
		names[m.ID] = m
	}

	for _, name := range []string{"TotalMemory", "FreeMemory", "CPUutilization1"} {
		m, ok := names[name]
		require.True(t, ok, "метрика %s не попала в батч", name)
		assert.Equal(t, models.Gauge, m.MType)
		assert.NotNil(t, m.Value)
	}
}

func TestSnapshotTakesPollCountOnce(t *testing.T) {
	a := newAgent("localhost:8080", "")
	a.poll()
	a.poll()

	assert.Equal(t, int64(2), a.snapshot().delta)
	assert.Equal(t, int64(0), a.snapshot().delta, "один и тот же PollCount ушёл дважды")
}

func TestReport(t *testing.T) {
	srv, dump := newCollector()
	defer srv.Close()

	a := newAgent(srv.URL, "")
	a.poll()
	report(t, a)

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

	a := newAgent(srv.URL, "")
	a.poll()
	report(t, a)

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

func TestReportSignsBody(t *testing.T) {
	srv, dump := newCollector()
	defer srv.Close()

	a := newAgent(srv.URL, "секрет")
	a.poll()
	report(t, a)

	got := dump()
	require.Len(t, got, 1)

	signature := got[0].headers.Get(hash.Header)
	require.NotEmpty(t, signature, "агент не подписал запрос")

	zr, err := gzip.NewReader(bytes.NewReader(got[0].raw))
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)

	assert.True(t, hash.Valid(body, "секрет", signature), "подпись не сходится с несжатым телом")
}

func TestReportWithoutKeyDoesNotSign(t *testing.T) {
	srv, dump := newCollector()
	defer srv.Close()

	a := newAgent(srv.URL, "")
	a.poll()
	report(t, a)

	got := dump()
	require.Len(t, got, 1)
	assert.Empty(t, got[0].headers.Get(hash.Header))
}

func TestAgentSendsSignedToRealServer(t *testing.T) {
	store := storage.NewMemStorage()
	srv := httptest.NewServer(handler.New(store, nil, zap.NewNop()).Router(zap.NewNop(), "секрет"))
	defer srv.Close()

	delta := int64(3)
	a := newAgent(srv.URL, "секрет")
	require.NoError(t, a.send(t.Context(), []models.Metrics{{ID: "PollCount", MType: models.Counter, Delta: &delta}}))

	total, err := store.Counter(t.Context(), "PollCount")
	require.NoError(t, err, "сервер отверг подписанный запрос")
	assert.Equal(t, int64(3), total)
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

			a := newAgent(srv.URL, "")
			require.NoError(t, a.send(t.Context(), []models.Metrics{c.metric}))

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

	a := newAgent(srv.URL, "")
	a.poll()
	a.poll()
	report(t, a)

	if a.pollCount.Load() != 0 {
		t.Errorf("после report pollCount = %d, хотел 0", a.pollCount.Load())
	}
}

func TestReportKeepsPollCountOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := newAgent(srv.URL, "")
	a.poll()
	a.poll()
	report(t, a)

	assert.Equal(t, int64(2), a.pollCount.Load(), "при ошибке отправки счётчик опросов терять нельзя")
}

func TestRetriableSend(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"сервер недоступен", &url.Error{Op: "Post", URL: "http://localhost:8080/updates/", Err: &net.OpError{Op: "dial", Err: errors.New("соединение отклонено")}}, true},
		{"сервер ответил ошибкой", errors.New("сервер ответил 500 Internal Server Error"), false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, retriableSend(c.err))
		})
	}
}

func TestSendReturnsErrorOnBadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	value := 1.0
	a := newAgent(srv.URL, "")

	err := a.send(t.Context(), []models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &value}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestAgentSendsToRealServer(t *testing.T) {
	store := storage.NewMemStorage()
	srv := httptest.NewServer(handler.New(store, nil, zap.NewNop()).Router(zap.NewNop(), ""))
	defer srv.Close()

	a := newAgent(srv.URL, "")
	a.poll()
	report(t, a)

	ctx := t.Context()

	v, err := store.Gauge(ctx, "Alloc")
	require.NoError(t, err, "сервер не сохранил Alloc")
	assert.Positive(t, v)

	d, err := store.Counter(ctx, "PollCount")
	require.NoError(t, err, "сервер не сохранил PollCount")
	assert.Equal(t, int64(1), d)
}

func TestWorkersRespectRateLimit(t *testing.T) {
	const limit = 3

	var mu sync.Mutex
	var active, peak int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		active++
		if active > peak {
			peak = active
		}
		mu.Unlock()

		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		active--
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := New(config.Agent{
		Addr:           strings.TrimPrefix(srv.URL, "http://"),
		ReportInterval: 1,
		PollInterval:   1,
		RateLimit:      limit,
	}, zap.NewNop())

	transport, ok := a.client.Transport.(*http.Transport)
	require.True(t, ok, "без транспорта не снять его лимит и параллелизм режет он, а не пул воркеров")
	transport.MaxConnsPerHost = 0

	jobs := make(chan reportJob, 9)
	for range cap(jobs) {
		jobs <- gaugeReportJob()
	}
	close(jobs)

	a.runWorkers(t.Context(), jobs)

	mu.Lock()
	defer mu.Unlock()

	assert.LessOrEqual(t, peak, limit, "запросов в полёте больше лимита")
	assert.Greater(t, peak, 1, "воркеры отправляли по очереди")
}

func TestNewLimitsConnectionsPerHost(t *testing.T) {
	a := New(config.Agent{Addr: "localhost:8080", ReportInterval: 10, PollInterval: 2, RateLimit: 5}, zap.NewNop())

	transport, ok := a.client.Transport.(*http.Transport)
	require.True(t, ok)
	assert.Equal(t, 5, transport.MaxConnsPerHost, "лимит запросов не дошёл до транспорта")
}

func TestNewFallsBackToSingleWorker(t *testing.T) {
	a := New(config.Agent{Addr: "localhost:8080", ReportInterval: 10, PollInterval: 2}, zap.NewNop())
	assert.Equal(t, 1, a.rateLimit)
}

func TestSplitFeedsEveryWorker(t *testing.T) {
	src := batchOf(51)
	parts := split(src, 4)

	require.Len(t, parts, 4, "батч не разошёлся по воркерам")

	var total, counters int
	var delta int64

	for _, p := range parts {
		total += len(p.batch)
		delta += p.delta

		for _, m := range p.batch {
			if m.MType == models.Counter {
				counters++
				assert.Equal(t, src.delta, p.delta, "дельта не у той части, что несёт PollCount")
			}
		}
	}

	assert.Equal(t, len(src.batch), total, "метрики потерялись при нарезке")
	assert.Equal(t, 1, counters, "PollCount попал больше чем в одну часть")
	assert.Equal(t, src.delta, delta, "дельта задвоилась или потерялась")
}

func TestSnapshotStartsWithPollCount(t *testing.T) {
	a := newAgent("localhost:8080", "")
	a.poll()
	a.pollSystem()

	batch := a.snapshot().batch
	require.NotEmpty(t, batch)

	first := batch[0]
	assert.Equal(t, "PollCount", first.ID, "split отдаёт дельту первой части, PollCount обязан идти первым")
	assert.Equal(t, models.Counter, first.MType)
}

func TestSplitKeepsOneJobForOneWorker(t *testing.T) {
	src := batchOf(51)
	parts := split(src, 1)

	require.Len(t, parts, 1)
	assert.Len(t, parts[0].batch, len(src.batch))
	assert.Equal(t, src.delta, parts[0].delta)
}

func TestSplitDoesNotCutBatchTooFine(t *testing.T) {
	parts := split(batchOf(15), 8)

	require.Len(t, parts, 2, "мелкий батч раздробили по числу воркеров")
	for _, p := range parts {
		assert.GreaterOrEqual(t, len(p.batch), 7)
	}
}

func TestReportLoopSplitsBatchIntoJobs(t *testing.T) {
	a := newAgent("localhost:8080", "")
	a.rateLimit = 4
	a.reportInterval = 10 * time.Millisecond
	a.poll()

	jobs := make(chan reportJob, 2*a.rateLimit)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.reportLoop(ctx, jobs)
	}()

	var sent, total, counters int
	for total < 29 {
		select {
		case j := <-jobs:
			sent++
			total += len(j.batch)
			for _, m := range j.batch {
				if m.MType == models.Counter {
					counters++
				}
			}
		case <-time.After(time.Second):
			t.Fatal("reportLoop не отправил весь батч")
		}
	}

	cancel()
	<-done

	assert.Greater(t, sent, 1, "тик уехал одним заданием, воркеры простаивают")
	assert.Equal(t, 29, total)
	assert.Equal(t, 1, counters)
}

func TestReportLoopWaitsForQueueAndKeepsPollCount(t *testing.T) {
	a := newAgent("localhost:8080", "")
	a.reportInterval = 10 * time.Millisecond
	a.poll()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.reportLoop(ctx, make(chan reportJob))
	}()

	require.Eventually(t, func() bool { return a.pollCount.Load() == 0 }, time.Second, time.Millisecond, "reportLoop не снял снапшот")

	select {
	case <-done:
		t.Fatal("reportLoop не стал ждать места в очереди")
	default:
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reportLoop не вышел по отмене контекста")
	}

	assert.Equal(t, int64(1), a.pollCount.Load(), "дельта не дождавшейся очереди части потерялась")
}

func TestRunStopsOnCancel(t *testing.T) {
	a := newAgent("127.0.0.1:1", "")
	a.pollInterval = 10 * time.Millisecond
	a.reportInterval = 10 * time.Millisecond

	before := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Run(ctx)
	}()

	require.Eventually(t, func() bool { return a.pollCount.Load() > 2 }, time.Second, 10*time.Millisecond, "агент не собирает метрики")
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("агент не остановился по отмене контекста")
	}

	time.Sleep(100 * time.Millisecond)
	assert.LessOrEqual(t, runtime.NumGoroutine(), before, "остались висящие горутины")
}
