package storage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
)

type FileStore struct {
	mu   sync.Mutex
	repo Repository
	path string
	log  *zap.Logger
}

func NewFileStore(repo Repository, path string, restore bool, log *zap.Logger) (*FileStore, error) {
	f := &FileStore{repo: repo, path: path, log: log}
	if !restore {
		return f, nil
	}
	return f, f.load()
}

func (f *FileStore) load() error {
	if f.path == "" {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	var metrics []models.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return err
	}

	for _, m := range metrics {
		switch m.MType {
		case models.Gauge:
			if m.Value != nil {
				f.repo.UpdateGauge(m.ID, *m.Value)
			}
		case models.Counter:
			if m.Delta != nil {
				f.repo.AddCounter(m.ID, *m.Delta)
			}
		}
	}

	return nil
}

func (f *FileStore) save() error {
	if f.path == "" {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	gauges, counters := f.repo.Snapshot()

	metrics := make([]models.Metrics, 0, len(gauges)+len(counters))
	for name, v := range gauges {
		metrics = append(metrics, models.Metrics{ID: name, MType: models.Gauge, Value: &v})
	}
	for name, v := range counters {
		metrics = append(metrics, models.Metrics{ID: name, MType: models.Counter, Delta: &v})
	}

	data, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	if dir := filepath.Dir(f.path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o666); err != nil {
		return err
	}

	return os.Rename(tmp, f.path)
}

func (f *FileStore) Close() error {
	return f.save()
}

func (f *FileStore) RunPeriodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := f.save(); err != nil {
				f.log.Warn("не сохранил метрики", zap.Error(err))
			}
		case <-ctx.Done():
			return
		}
	}
}

type syncRepository struct {
	Repository
	file *FileStore
	log  *zap.Logger
}

func (f *FileStore) SyncRepository() Repository {
	return &syncRepository{Repository: f.repo, file: f, log: f.log}
}

func (s *syncRepository) UpdateGauge(name string, value float64) {
	s.Repository.UpdateGauge(name, value)
	s.save()
}

func (s *syncRepository) AddCounter(name string, delta int64) {
	s.Repository.AddCounter(name, delta)
	s.save()
}

func (s *syncRepository) save() {
	if err := s.file.save(); err != nil {
		s.log.Warn("не сохранил метрики", zap.Error(err))
	}
}
