package agent

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"strconv"
	"time"

	models "github.com/mgfan1/go-metrics/internal/model"
)

type Agent struct {
	baseURL        string
	pollInterval   time.Duration
	reportInterval time.Duration
	client         *http.Client
	gauges         map[string]float64
	pollCount      int64
}

func New(serverAddr string, poll, report time.Duration) *Agent {
	return &Agent{
		baseURL:        "http://" + serverAddr,
		pollInterval:   poll,
		reportInterval: report,
		client:         &http.Client{Timeout: 5 * time.Second},
		gauges:         make(map[string]float64),
	}
}

func (a *Agent) Run() {
	pollTicker := time.NewTicker(a.pollInterval)
	reportTicker := time.NewTicker(a.reportInterval)
	defer pollTicker.Stop()
	defer reportTicker.Stop()

	for {
		select {
		case <-pollTicker.C:
			a.poll()
		case <-reportTicker.C:
			a.report()
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

func (a *Agent) report() {
	for name, value := range a.gauges {
		a.send(models.Gauge, name, strconv.FormatFloat(value, 'f', -1, 64))
	}

	a.send(models.Counter, "PollCount", strconv.FormatInt(a.pollCount, 10))
	a.pollCount = 0
}

func (a *Agent) send(mType, name, value string) {
	url := fmt.Sprintf("%s/update/%s/%s/%s", a.baseURL, mType, name, value)

	req, err := http.NewRequest(http.MethodPost, url, http.NoBody)
	if err != nil {
		log.Printf("agent: не собрал запрос для %s: %v", name, err)
		return
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := a.client.Do(req)
	if err != nil {
		log.Printf("agent: не отправил %s: %v", name, err)
		return
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
