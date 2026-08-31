package storage

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
)

const defaultTestDSN = "postgres://metrics:metrics@localhost:5432/metrics?sslmode=disable"

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := defaultTestDSN
	if v, ok := os.LookupEnv("TEST_DATABASE_DSN"); ok {
		dsn = v
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("нет базы для теста: %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		db.Close()
		t.Skipf("нет базы для теста: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func newPGStorage(t *testing.T) *PGStorage {
	t.Helper()

	db := openTestDB(t)

	store, err := NewPGStorage(db, zap.NewNop())
	require.NoError(t, err)

	_, err = db.ExecContext(context.Background(), "TRUNCATE TABLE metrics")
	require.NoError(t, err)

	return store
}

func TestPGGaugeReplaces(t *testing.T) {
	ctx := context.Background()
	s := newPGStorage(t)

	require.NoError(t, s.UpdateGauge(ctx, "Alloc", 100.5))
	require.NoError(t, s.UpdateGauge(ctx, "Alloc", 42.1))

	got, err := s.Gauge(ctx, "Alloc")
	require.NoError(t, err)
	assert.Equal(t, 42.1, got)
}

func TestPGCounterAccumulates(t *testing.T) {
	ctx := context.Background()
	s := newPGStorage(t)

	require.NoError(t, s.AddCounter(ctx, "PollCount", 5))
	require.NoError(t, s.AddCounter(ctx, "PollCount", 3))

	got, err := s.Counter(ctx, "PollCount")
	require.NoError(t, err)
	assert.Equal(t, int64(8), got)
}

func TestPGMissingMetric(t *testing.T) {
	ctx := context.Background()
	s := newPGStorage(t)

	_, err := s.Gauge(ctx, "nope")
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = s.Counter(ctx, "nope")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPGSnapshot(t *testing.T) {
	ctx := context.Background()
	s := newPGStorage(t)

	require.NoError(t, s.UpdateGauge(ctx, "Alloc", 13.5))
	require.NoError(t, s.AddCounter(ctx, "PollCount", 7))

	gauges, counters, err := s.Snapshot(ctx)
	require.NoError(t, err)

	assert.Equal(t, map[string]float64{"Alloc": 13.5}, gauges)
	assert.Equal(t, map[string]int64{"PollCount": 7}, counters)
}

func TestPGSameNameDifferentTypes(t *testing.T) {
	ctx := context.Background()
	s := newPGStorage(t)

	require.NoError(t, s.UpdateGauge(ctx, "Same", 1.5))
	require.NoError(t, s.AddCounter(ctx, "Same", 2))

	gauge, err := s.Gauge(ctx, "Same")
	require.NoError(t, err)
	assert.Equal(t, 1.5, gauge)

	counter, err := s.Counter(ctx, "Same")
	require.NoError(t, err)
	assert.Equal(t, int64(2), counter)
}

func TestPGUpdateBatchWithDuplicates(t *testing.T) {
	ctx := context.Background()
	s := newPGStorage(t)

	batch := []models.Metrics{
		counter("PollCount", 5),
		gauge("Alloc", 1.5),
		counter("PollCount", 3),
		gauge("Alloc", 42.1),
	}
	require.NoError(t, s.UpdateBatch(ctx, batch), "дубликаты ID в батче не должны ломать транзакцию")

	got, err := s.Counter(ctx, "PollCount")
	require.NoError(t, err)
	assert.Equal(t, int64(8), got)

	value, err := s.Gauge(ctx, "Alloc")
	require.NoError(t, err)
	assert.Equal(t, 42.1, value)
}

func TestPGUpdateBatchEmpty(t *testing.T) {
	ctx := context.Background()
	s := newPGStorage(t)

	require.NoError(t, s.UpdateBatch(ctx, nil))

	gauges, counters, err := s.Snapshot(ctx)
	require.NoError(t, err)
	assert.Empty(t, gauges)
	assert.Empty(t, counters)
}

func TestPGUpdateBatchRollsBackOnError(t *testing.T) {
	ctx := context.Background()
	s := newPGStorage(t)

	batch := []models.Metrics{
		gauge("Alloc", 1.5),
		gauge(strings.Repeat("x", 300), 2.5),
	}
	require.Error(t, s.UpdateBatch(ctx, batch), "имя длиннее varchar(255) должно ломать батч")

	_, err := s.Gauge(ctx, "Alloc")
	assert.ErrorIs(t, err, ErrNotFound, "метрика из упавшего батча не должна остаться в таблице")
}

func TestPGMigrationsAreIdempotent(t *testing.T) {
	newPGStorage(t)

	db := openTestDB(t)
	_, err := NewPGStorage(db, zap.NewNop())
	assert.NoError(t, err, "повторный запуск миграций не должен быть ошибкой")
}
