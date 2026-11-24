package storage

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
)

func benchBatch() []models.Metrics {
	delta := int64(5)
	batch := []models.Metrics{{ID: "PollCount", MType: models.Counter, Delta: &delta}}
	for i := range 34 {
		value := float64(i) * 1234.5678
		batch = append(batch, models.Metrics{ID: "Gauge" + strconv.Itoa(i), MType: models.Gauge, Value: &value})
	}
	return batch
}

func BenchmarkMemStorageUpdateBatch(b *testing.B) {
	s := NewMemStorage()
	batch := benchBatch()
	ctx := b.Context()

	for b.Loop() {
		if err := s.UpdateBatch(ctx, batch); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMemStorageSnapshot(b *testing.B) {
	s := NewMemStorage()
	ctx := b.Context()
	require.NoError(b, s.UpdateBatch(ctx, benchBatch()))

	for b.Loop() {
		if _, _, err := s.Snapshot(ctx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFileStoreSave(b *testing.B) {
	s := NewMemStorage()
	ctx := b.Context()
	require.NoError(b, s.UpdateBatch(ctx, benchBatch()))

	f, err := NewFileStore(s, filepath.Join(b.TempDir(), "metrics.json"), false, zap.NewNop())
	require.NoError(b, err)

	for b.Loop() {
		if err := f.save(ctx); err != nil {
			b.Fatal(err)
		}
	}
}
