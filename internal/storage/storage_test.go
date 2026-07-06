package storage

import "testing"

func TestGaugeReplaces(t *testing.T) {
	s := NewMemStorage()
	s.UpdateGauge("Alloc", 100.5)
	s.UpdateGauge("Alloc", 42.1)

	got, ok := s.Gauge("Alloc")
	if !ok || got != 42.1 {
		t.Fatalf("gauge = %v, %v; хотел 42.1, true", got, ok)
	}
}

func TestCounterAccumulates(t *testing.T) {
	s := NewMemStorage()
	s.AddCounter("PollCount", 5)
	s.AddCounter("PollCount", 3)

	got, ok := s.Counter("PollCount")
	if !ok || got != 8 {
		t.Fatalf("counter = %v, %v; хотел 8, true", got, ok)
	}
}

func TestMissingMetric(t *testing.T) {
	s := NewMemStorage()
	if _, ok := s.Gauge("nope"); ok {
		t.Error("для отсутствующего gauge ждал ok=false")
	}
	if _, ok := s.Counter("nope"); ok {
		t.Error("для отсутствующего counter ждал ok=false")
	}
}

func TestSnapshotIsCopy(t *testing.T) {
	s := NewMemStorage()
	s.UpdateGauge("A", 1)

	gauges, _ := s.Snapshot()
	gauges["A"] = 999

	if got, _ := s.Gauge("A"); got != 1 {
		t.Errorf("Snapshot вернул ссылку на внутреннюю карту: got %v", got)
	}
}
