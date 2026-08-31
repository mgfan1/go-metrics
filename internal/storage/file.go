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
	return f, f.load(context.Background())
}

func (f *FileStore) load(ctx context.Context) error {
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
				if err := f.repo.UpdateGauge(ctx, m.ID, *m.Value); err != nil {
					return err
				}
			}
		case models.Counter:
			if m.Delta != nil {
				if err := f.repo.AddCounter(ctx, m.ID, *m.Delta); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (f *FileStore) save(ctx context.Context) error {
	if f.path == "" {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	gauges, counters, err := f.repo.Snapshot(ctx)
	if err != nil {
		return err
	}

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
	return f.save(context.Background())
}

func (f *FileStore) RunPeriodic(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := f.save(ctx); err != nil {
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

func (s *syncRepository) UpdateGauge(ctx context.Context, name string, value float64) error {
	if err := s.Repository.UpdateGauge(ctx, name, value); err != nil {
		return err
	}
	s.save(ctx)
	return nil
}

func (s *syncRepository) AddCounter(ctx context.Context, name string, delta int64) error {
	if err := s.Repository.AddCounter(ctx, name, delta); err != nil {
		return err
	}
	s.save(ctx)
	return nil
}

func (s *syncRepository) save(ctx context.Context) {
	if err := s.file.save(ctx); err != nil {
		s.log.Warn("не сохранил метрики", zap.Error(err))
	}
}
