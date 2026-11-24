package storage

import (
	"context"
	"errors"
	"sync"

	models "github.com/mgfan1/go-metrics/internal/model"
)

// ErrNotFound означает, что метрики с таким именем нет.
var ErrNotFound = errors.New("метрика не найдена")

// Repository описывает хранилище метрик.
type Repository interface {
	// UpdateGauge сохраняет значение gauge.
	UpdateGauge(ctx context.Context, name string, value float64) error
	// AddCounter прибавляет delta к counter.
	AddCounter(ctx context.Context, name string, delta int64) error
	// UpdateBatch применяет батч метрик целиком.
	UpdateBatch(ctx context.Context, metrics []models.Metrics) error
	// Gauge возвращает значение gauge или ErrNotFound.
	Gauge(ctx context.Context, name string) (float64, error)
	// Counter возвращает значение counter или ErrNotFound.
	Counter(ctx context.Context, name string) (int64, error)
	// Snapshot возвращает копии всех gauge и counter.
	Snapshot(ctx context.Context) (gauges map[string]float64, counters map[string]int64, err error)
}

// MemStorage хранит метрики в памяти и безопасен для конкурентного доступа.
type MemStorage struct {
	mu       sync.RWMutex
	gauges   map[string]float64
	counters map[string]int64
}

// NewMemStorage создаёт пустое хранилище.
func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

// UpdateGauge сохраняет значение gauge.
func (s *MemStorage) UpdateGauge(_ context.Context, name string, value float64) error {
	s.mu.Lock()
	s.gauges[name] = value
	s.mu.Unlock()
	return nil
}

// AddCounter прибавляет delta к counter.
func (s *MemStorage) AddCounter(_ context.Context, name string, delta int64) error {
	s.mu.Lock()
	s.counters[name] += delta
	s.mu.Unlock()
	return nil
}

// UpdateBatch применяет батч метрик целиком.
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

// Gauge возвращает значение gauge или ErrNotFound.
func (s *MemStorage) Gauge(_ context.Context, name string) (float64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.gauges[name]
	if !ok {
		return 0, ErrNotFound
	}
	return v, nil
}

// Counter возвращает значение counter или ErrNotFound.
func (s *MemStorage) Counter(_ context.Context, name string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.counters[name]
	if !ok {
		return 0, ErrNotFound
	}
	return v, nil
}

// Snapshot возвращает копии всех gauge и counter.
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
