package storage

import "sync"

type Repository interface {
	UpdateGauge(name string, value float64)
	AddCounter(name string, delta int64)
	Gauge(name string) (float64, bool)
	Counter(name string) (int64, bool)
	Snapshot() (gauges map[string]float64, counters map[string]int64)
}

type MemStorage struct {
	mu       sync.RWMutex
	gauges   map[string]float64
	counters map[string]int64
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (s *MemStorage) UpdateGauge(name string, value float64) {
	s.mu.Lock()
	s.gauges[name] = value
	s.mu.Unlock()
}

func (s *MemStorage) AddCounter(name string, delta int64) {
	s.mu.Lock()
	s.counters[name] += delta
	s.mu.Unlock()
}

func (s *MemStorage) Gauge(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.gauges[name]
	return v, ok
}

func (s *MemStorage) Counter(name string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.counters[name]
	return v, ok
}

func (s *MemStorage) Snapshot() (map[string]float64, map[string]int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	gauges := make(map[string]float64, len(s.gauges))
	for k, v := range s.gauges {
		gauges[k] = v
	}
	counters := make(map[string]int64, len(s.counters))
	for k, v := range s.counters {
		counters[k] = v
	}
	return gauges, counters
}
