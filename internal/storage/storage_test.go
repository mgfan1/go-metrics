package storage

import (
	"context"
	"errors"
	"testing"
)

func TestGaugeReplaces(t *testing.T) {
	ctx := context.Background()
	s := NewMemStorage()
	s.UpdateGauge(ctx, "Alloc", 100.5)
	s.UpdateGauge(ctx, "Alloc", 42.1)

	got, err := s.Gauge(ctx, "Alloc")
	if err != nil || got != 42.1 {
		t.Fatalf("gauge = %v, %v; хотел 42.1, nil", got, err)
	}
}

func TestCounterAccumulates(t *testing.T) {
	ctx := context.Background()
	s := NewMemStorage()
	s.AddCounter(ctx, "PollCount", 5)
	s.AddCounter(ctx, "PollCount", 3)

	got, err := s.Counter(ctx, "PollCount")
	if err != nil || got != 8 {
		t.Fatalf("counter = %v, %v; хотел 8, nil", got, err)
	}
}

func TestMissingMetric(t *testing.T) {
	ctx := context.Background()
	s := NewMemStorage()

	if _, err := s.Gauge(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("для отсутствующего gauge ждал ErrNotFound, получил %v", err)
	}
	if _, err := s.Counter(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("для отсутствующего counter ждал ErrNotFound, получил %v", err)
	}
}

func TestSnapshotIsCopy(t *testing.T) {
	ctx := context.Background()
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
