package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/config"
	"github.com/mgfan1/go-metrics/internal/hash"
	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/internal/retry"
)

const minPartSize = 10

type reportJob struct {
	batch []models.Metrics
	delta int64
}

type Agent struct {
	baseURL        string
	pollInterval   time.Duration
	reportInterval time.Duration
	signKey        string
	rateLimit      int
	client         *http.Client
	mu             sync.RWMutex
	gauges         map[string]float64
	pollCount      atomic.Int64
	log            *zap.Logger
	retrier        *retry.Retrier
}

func New(cfg config.Agent, log *zap.Logger) *Agent {
	rateLimit := max(cfg.RateLimit, 1)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = rateLimit
	transport.MaxIdleConnsPerHost = rateLimit
	transport.MaxConnsPerHost = rateLimit

	return &Agent{
		baseURL:        "http://" + cfg.Addr,
		pollInterval:   time.Duration(cfg.PollInterval) * time.Second,
		reportInterval: time.Duration(cfg.ReportInterval) * time.Second,
		signKey:        cfg.Key,
		rateLimit:      rateLimit,
		client:         &http.Client{Timeout: 5 * time.Second, Transport: transport},
		gauges:         make(map[string]float64),
		log:            log,
		retrier:        retry.New(log),
	}
}

func (a *Agent) Run(ctx context.Context) {
	a.poll()
	a.pollSystem()

	jobs := make(chan reportJob, 2*a.rateLimit)

	var pool sync.WaitGroup
	pool.Go(func() { a.runWorkers(ctx, jobs) })

	var collectors sync.WaitGroup
	collectors.Go(func() { a.pollLoop(ctx, a.poll) })
	collectors.Go(func() { a.pollLoop(ctx, a.pollSystem) })
	collectors.Go(func() { a.reportLoop(ctx, jobs) })
	collectors.Wait()

	close(jobs)
	pool.Wait()

	a.client.CloseIdleConnections()
}

func (a *Agent) pollLoop(ctx context.Context, collect func()) {
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			collect()
		}
	}
}

func (a *Agent) reportLoop(ctx context.Context, jobs chan<- reportJob) {
	ticker := time.NewTicker(a.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, j := range split(a.snapshot(), a.rateLimit) {
				select {
				case jobs <- j:
				case <-ctx.Done():
					a.pollCount.Add(j.delta)
					return
				}
			}
		}
	}
}

func (a *Agent) runWorkers(ctx context.Context, jobs <-chan reportJob) {
	var wg sync.WaitGroup

	for i := range a.rateLimit {
		log := a.log.With(zap.Int("worker", i+1))
		wg.Go(func() {
			for j := range jobs {
				a.deliver(ctx, j, log)
			}
		})
	}

	wg.Wait()
}

func (a *Agent) poll() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	a.mu.Lock()
	a.gauges["Alloc"] = float64(m.Alloc)
	a.gauges["BuckHashSys"] = float64(m.BuckHashSys)
	a.gauges["Frees"] = float64(m.Frees)
	a.gauges["GCCPUFraction"] = m.GCCPUFraction
	a.gauges["GCSys"] = float64(m.GCSys)
	a.gauges["HeapAlloc"] = float64(m.HeapAlloc)
	a.gauges["HeapIdle"] = float64(m.HeapIdle)
	a.gauges["HeapInuse"] = float64(m.HeapInuse)
	a.gauges["HeapObjects"] = float64(m.HeapObjects)
	a.gauges["HeapReleased"] = float64(m.HeapReleased)
	a.gauges["HeapSys"] = float64(m.HeapSys)
	a.gauges["LastGC"] = float64(m.LastGC)
	a.gauges["Lookups"] = float64(m.Lookups)
	a.gauges["MCacheInuse"] = float64(m.MCacheInuse)
	a.gauges["MCacheSys"] = float64(m.MCacheSys)
	a.gauges["MSpanInuse"] = float64(m.MSpanInuse)
	a.gauges["MSpanSys"] = float64(m.MSpanSys)
	a.gauges["Mallocs"] = float64(m.Mallocs)
	a.gauges["NextGC"] = float64(m.NextGC)
	a.gauges["NumForcedGC"] = float64(m.NumForcedGC)
	a.gauges["NumGC"] = float64(m.NumGC)
	a.gauges["OtherSys"] = float64(m.OtherSys)
	a.gauges["PauseTotalNs"] = float64(m.PauseTotalNs)
	a.gauges["StackInuse"] = float64(m.StackInuse)
	a.gauges["StackSys"] = float64(m.StackSys)
	a.gauges["Sys"] = float64(m.Sys)
	a.gauges["TotalAlloc"] = float64(m.TotalAlloc)
	a.gauges["RandomValue"] = rand.Float64()
	a.mu.Unlock()

	a.pollCount.Add(1)
}

func (a *Agent) pollSystem() {
	collected := make(map[string]float64, 3)

	if vm, err := mem.VirtualMemory(); err != nil {
		a.log.Warn("не собрал метрики памяти", zap.Error(err))
	} else {
		collected["TotalMemory"] = float64(vm.Total)
		collected["FreeMemory"] = float64(vm.Free)
	}

	if loads, err := cpu.Percent(0, true); err != nil {
		a.log.Warn("не снял загрузку CPU", zap.Error(err))
	} else {
		for i, load := range loads {
			collected["CPUutilization"+strconv.Itoa(i+1)] = load
		}
	}

	if len(collected) == 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	for name, value := range collected {
		a.gauges[name] = value
	}
}

func (a *Agent) snapshot() reportJob {
	delta := a.pollCount.Swap(0)

	a.mu.RLock()
	batch := make([]models.Metrics, 0, len(a.gauges)+1)
	batch = append(batch, models.Metrics{ID: "PollCount", MType: models.Counter, Delta: &delta})
	for name, value := range a.gauges {
		batch = append(batch, models.Metrics{ID: name, MType: models.Gauge, Value: &value})
	}
	a.mu.RUnlock()

	return reportJob{batch: batch, delta: delta}
}

func split(j reportJob, parts int) []reportJob {
	if limit := (len(j.batch) + minPartSize - 1) / minPartSize; parts > limit {
		parts = limit
	}
	if parts < 2 {
		return []reportJob{j}
	}

	size := (len(j.batch) + parts - 1) / parts

	jobs := make([]reportJob, 0, parts)
	for start := 0; start < len(j.batch); start += size {
		jobs = append(jobs, reportJob{batch: j.batch[start:min(start+size, len(j.batch))]})
	}
	jobs[0].delta = j.delta

	return jobs
}

func (a *Agent) deliver(ctx context.Context, j reportJob, log *zap.Logger) {
	err := a.retrier.Do(ctx, retriableSend, func() error { return a.send(ctx, j.batch) })
	if err == nil {
		return
	}

	a.pollCount.Add(j.delta)

	if ctx.Err() != nil {
		log.Debug("не отправил метрики при остановке", zap.Int("count", len(j.batch)), zap.Error(err))
		return
	}

	log.Warn("не отправил метрики", zap.Int("count", len(j.batch)), zap.Error(err))
}

func retriableSend(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr)
}

func (a *Agent) send(ctx context.Context, batch []models.Metrics) error {
	body, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(body); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/updates/", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	if a.signKey != "" {
		req.Header.Set(hash.Header, hash.Sign(body, a.signKey))
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("сервер ответил %s", resp.Status)
	}

	return nil
}
