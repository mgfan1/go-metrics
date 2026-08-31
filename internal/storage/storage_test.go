package storage

import (
	"errors"
	"sync"
	"testing"

	models "github.com/mgfan1/go-metrics/internal/model"
)

func gauge(id string, v float64) models.Metrics {
	return models.Metrics{ID: id, MType: models.Gauge, Value: &v}
}

func counter(id string, d int64) models.Metrics {
	return models.Metrics{ID: id, MType: models.Counter, Delta: &d}
}

func TestGaugeReplaces(t *testing.T) {
	ctx := t.Context()
	s := NewMemStorage()
	s.UpdateGauge(ctx, "Alloc", 100.5)
	s.UpdateGauge(ctx, "Alloc", 42.1)

	got, err := s.Gauge(ctx, "Alloc")
	if err != nil || got != 42.1 {
		t.Fatalf("gauge = %v, %v; хотел 42.1, nil", got, err)
	}
}

func TestCounterAccumulates(t *testing.T) {
	ctx := t.Context()
	s := NewMemStorage()
	s.AddCounter(ctx, "PollCount", 5)
	s.AddCounter(ctx, "PollCount", 3)

	got, err := s.Counter(ctx, "PollCount")
	if err != nil || got != 8 {
		t.Fatalf("counter = %v, %v; хотел 8, nil", got, err)
	}
}

func TestMissingMetric(t *testing.T) {
	ctx := t.Context()
	s := NewMemStorage()

	if _, err := s.Gauge(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("для отсутствующего gauge ждал ErrNotFound, получил %v", err)
	}
	if _, err := s.Counter(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("для отсутствующего counter ждал ErrNotFound, получил %v", err)
	}
}

func TestUpdateBatchWithDuplicates(t *testing.T) {
	ctx := t.Context()
	s := NewMemStorage()

	batch := []models.Metrics{
		counter("PollCount", 5),
		gauge("Alloc", 1.5),
		counter("PollCount", 3),
		gauge("Alloc", 42.1),
	}
	if err := s.UpdateBatch(ctx, batch); err != nil {
		t.Fatalf("UpdateBatch: %v", err)
	}

	if got, _ := s.Counter(ctx, "PollCount"); got != 8 {
		t.Errorf("counter = %d, хотел 8", got)
	}
	if got, _ := s.Gauge(ctx, "Alloc"); got != 42.1 {
		t.Errorf("gauge = %v, хотел 42.1", got)
	}
}

func TestUpdateBatchEmpty(t *testing.T) {
	ctx := t.Context()
	s := NewMemStorage()

	if err := s.UpdateBatch(ctx, nil); err != nil {
		t.Fatalf("пустой батч не должен быть ошибкой: %v", err)
	}

	gauges, counters, _ := s.Snapshot(ctx)
	if len(gauges) != 0 || len(counters) != 0 {
		t.Error("хранилище должно остаться пустым")
	}
}

func TestUpdateBatchSkipsMetricsWithoutValue(t *testing.T) {
	ctx := t.Context()
	s := NewMemStorage()

	batch := []models.Metrics{
		{ID: "Alloc", MType: models.Gauge},
		{ID: "PollCount", MType: models.Counter},
		gauge("Sys", 2),
	}
	if err := s.UpdateBatch(ctx, batch); err != nil {
		t.Fatalf("UpdateBatch: %v", err)
	}

	if _, err := s.Gauge(ctx, "Alloc"); !errors.Is(err, ErrNotFound) {
		t.Error("gauge без значения записывать нельзя")
	}
	if got, err := s.Gauge(ctx, "Sys"); err != nil || got != 2 {
		t.Errorf("gauge = %v, %v; хотел 2, nil", got, err)
	}
}

func TestUpdateBatchConcurrent(t *testing.T) {
	ctx := t.Context()
	s := NewMemStorage()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s.UpdateBatch(ctx, []models.Metrics{counter("PollCount", 1), gauge("Alloc", 1)})
			}
		}()
	}
	wg.Wait()

	if got, _ := s.Counter(ctx, "PollCount"); got != 400 {
		t.Errorf("counter = %d, хотел 400", got)
	}
}

func TestSnapshotIsCopy(t *testing.T) {
	ctx := t.Context()
	s := NewMemStorage()
	s.UpdateGauge(ctx, "A", 1)

	gauges, _, err := s.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	gauges["A"] = 999

	if got, _ := s.Gauge(ctx, "A"); got != 1 {
		t.Errorf("Snapshot вернул ссылку на внутреннюю карту: got %v", got)
	}
}
