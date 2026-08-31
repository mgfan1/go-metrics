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
	"time"

	"go.uber.org/zap"

	"github.com/mgfan1/go-metrics/internal/config"
	"github.com/mgfan1/go-metrics/internal/hash"
	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/internal/retry"
)

type Agent struct {
	baseURL        string
	pollInterval   time.Duration
	reportInterval time.Duration
	key            string
	client         *http.Client
	gauges         map[string]float64
	pollCount      int64
	log            *zap.Logger
	retrier        *retry.Retrier
}

func New(cfg config.Agent, log *zap.Logger) *Agent {
	return &Agent{
		baseURL:        "http://" + cfg.Addr,
		pollInterval:   time.Duration(cfg.PollInterval) * time.Second,
		reportInterval: time.Duration(cfg.ReportInterval) * time.Second,
		key:            cfg.Key,
		client:         &http.Client{Timeout: 5 * time.Second},
		gauges:         make(map[string]float64),
		log:            log,
		retrier:        retry.New(log),
	}
}

func (a *Agent) Run(ctx context.Context) {
	pollTicker := time.NewTicker(a.pollInterval)
	reportTicker := time.NewTicker(a.reportInterval)
	defer pollTicker.Stop()
	defer reportTicker.Stop()

	for {
		if ctx.Err() != nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-pollTicker.C:
			a.poll()
		case <-reportTicker.C:
			a.report(ctx)
		}
	}
}

func (a *Agent) poll() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

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

	a.pollCount++
}

func (a *Agent) report(ctx context.Context) {
	delta := a.pollCount

	batch := make([]models.Metrics, 0, len(a.gauges)+1)
	for name, value := range a.gauges {
		batch = append(batch, models.Metrics{ID: name, MType: models.Gauge, Value: &value})
	}
	batch = append(batch, models.Metrics{ID: "PollCount", MType: models.Counter, Delta: &delta})

	err := a.retrier.Do(ctx, retriableSend, func() error { return a.send(ctx, batch) })
	if err != nil {
		a.log.Warn("не отправил метрики", zap.Int("count", len(batch)), zap.Error(err))
		return
	}
	a.pollCount -= delta
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
	if a.key != "" {
		req.Header.Set(hash.Header, hash.Sign(body, a.key))
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
