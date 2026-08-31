package storage

import (
	"context"
	"errors"
	"sync"

	models "github.com/mgfan1/go-metrics/internal/model"
)

var ErrNotFound = errors.New("метрика не найдена")

type Repository interface {
	UpdateGauge(ctx context.Context, name string, value float64) error
	AddCounter(ctx context.Context, name string, delta int64) error
	UpdateBatch(ctx context.Context, metrics []models.Metrics) error
	Gauge(ctx context.Context, name string) (float64, error)
	Counter(ctx context.Context, name string) (int64, error)
	Snapshot(ctx context.Context) (gauges map[string]float64, counters map[string]int64, err error)
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

func (s *MemStorage) UpdateGauge(_ context.Context, name string, value float64) error {
	s.mu.Lock()
	s.gauges[name] = value
	s.mu.Unlock()
	return nil
}

func (s *MemStorage) AddCounter(_ context.Context, name string, delta int64) error {
	s.mu.Lock()
	s.counters[name] += delta
	s.mu.Unlock()
	return nil
}

func (s *MemStorage) UpdateBatch(_ context.Context, metrics []models.Metrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, m := range metrics {
		switch m.MType {
		case models.Gauge:
			if m.Value != nil {
				s.gauges[m.ID] = *m.Value
			}
		case models.Counter:
			if m.Delta != nil {
				s.counters[m.ID] += *m.Delta
			}
		}
	}

	return nil
}

func (s *MemStorage) Gauge(_ context.Context, name string) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.gauges[name]
	if !ok {
		return 0, ErrNotFound
	}
	return v, nil
}

func (s *MemStorage) Counter(_ context.Context, name string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.counters[name]
	if !ok {
		return 0, ErrNotFound
	}
	return v, nil
}

func (s *MemStorage) Snapshot(_ context.Context) (map[string]float64, map[string]int64, error) {
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
	return gauges, counters, nil
}
