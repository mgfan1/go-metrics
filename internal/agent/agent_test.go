package agent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPoll(t *testing.T) {
	a := New("localhost:8080", time.Second, time.Second)
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
	var mu sync.Mutex
	seen := make(map[string]bool)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := New(strings.TrimPrefix(srv.URL, "http://"), time.Second, time.Second)
	a.poll()
	a.report()

	mu.Lock()
	defer mu.Unlock()

	if len(seen) != 29 {
		t.Errorf("сервер получил %d запросов, хотел 29", len(seen))
	}
	if !hasPrefix(seen, "/update/gauge/Alloc/") {
		t.Error("не отправлен gauge Alloc")
	}
	if !hasPrefix(seen, "/update/counter/PollCount/") {
		t.Error("не отправлен counter PollCount")
	}
}

func TestReportResetsPollCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := New(strings.TrimPrefix(srv.URL, "http://"), time.Second, time.Second)
	a.poll()
	a.poll()
	a.report()

	if a.pollCount != 0 {
		t.Errorf("после report pollCount = %d, хотел 0", a.pollCount)
	}
}

func hasPrefix(paths map[string]bool, prefix string) bool {
	for p := range paths {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}
