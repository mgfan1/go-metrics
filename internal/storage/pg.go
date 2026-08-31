package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/migrations"
)

const (
	upsertGauge = `INSERT INTO metrics (id, mtype, value) VALUES ($1, 'gauge', $2)
	 ON CONFLICT (id, mtype) DO UPDATE SET value = EXCLUDED.value`
	upsertCounter = `INSERT INTO metrics (id, mtype, delta) VALUES ($1, 'counter', $2)
	 ON CONFLICT (id, mtype) DO UPDATE SET delta = metrics.delta + EXCLUDED.delta`
)

type PGStorage struct {
	db  *sql.DB
	log *zap.Logger
}

func NewPGStorage(db *sql.DB, log *zap.Logger) (*PGStorage, error) {
	if err := applyMigrations(db); err != nil {
		return nil, err
	}
	log.Info("схема метрик готова")

	return &PGStorage{db: db, log: log}, nil
}

func applyMigrations(db *sql.DB) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("не прочитал миграции: %w", err)
	}

	conn, err := db.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("не получил соединение для миграций: %w", err)
	}
	defer conn.Close()

	drv, err := postgres.WithConnection(context.Background(), conn, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("не подготовил драйвер миграций: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", drv)
	if err != nil {
		return fmt.Errorf("не создал мигратор: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("не применил миграции: %w", err)
	}

	return nil
}

func (s *PGStorage) UpdateGauge(ctx context.Context, name string, value float64) error {
	_, err := s.db.ExecContext(ctx, upsertGauge, name, value)
	return err
}

func (s *PGStorage) AddCounter(ctx context.Context, name string, delta int64) error {
	_, err := s.db.ExecContext(ctx, upsertCounter, name, delta)
	return err
}

func collapseBatch(metrics []models.Metrics) []models.Metrics {
	type key struct {
		id    string
		mtype string
	}

	latest := make(map[key]models.Metrics, len(metrics))
	for _, m := range metrics {
		k := key{id: m.ID, mtype: m.MType}

		switch m.MType {
		case models.Gauge:
			if m.Value == nil {
				continue
			}
			value := *m.Value
			latest[k] = models.Metrics{ID: m.ID, MType: m.MType, Value: &value}
		case models.Counter:
			if m.Delta == nil {
				continue
			}
			delta := *m.Delta
			if prev, ok := latest[k]; ok {
				delta += *prev.Delta
			}
			latest[k] = models.Metrics{ID: m.ID, MType: m.MType, Delta: &delta}
		}
	}

	collapsed := make([]models.Metrics, 0, len(latest))
	for _, m := range latest {
		collapsed = append(collapsed, m)
	}

	sort.Slice(collapsed, func(i, j int) bool {
		if collapsed[i].MType != collapsed[j].MType {
			return collapsed[i].MType < collapsed[j].MType
		}
		return collapsed[i].ID < collapsed[j].ID
	})

	return collapsed
}

func (s *PGStorage) UpdateBatch(ctx context.Context, metrics []models.Metrics) error {
	batch := collapseBatch(metrics)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("не начал транзакцию: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	gauge, err := tx.PrepareContext(ctx, upsertGauge)
	if err != nil {
		return err
	}
	defer gauge.Close()

	counter, err := tx.PrepareContext(ctx, upsertCounter)
	if err != nil {
		return err
	}
	defer counter.Close()

	for _, m := range batch {
		switch m.MType {
		case models.Gauge:
			if _, err := gauge.ExecContext(ctx, m.ID, *m.Value); err != nil {
				return err
			}
		case models.Counter:
			if _, err := counter.ExecContext(ctx, m.ID, *m.Delta); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *PGStorage) Gauge(ctx context.Context, name string) (float64, error) {
	var value sql.NullFloat64

	row := s.db.QueryRowContext(ctx, `SELECT value FROM metrics WHERE id = $1 AND mtype = 'gauge'`, name)
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}

	return value.Float64, nil
}

func (s *PGStorage) Counter(ctx context.Context, name string) (int64, error) {
	var delta sql.NullInt64

	row := s.db.QueryRowContext(ctx, `SELECT delta FROM metrics WHERE id = $1 AND mtype = 'counter'`, name)
	if err := row.Scan(&delta); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}

	return delta.Int64, nil
}

func (s *PGStorage) Snapshot(ctx context.Context) (map[string]float64, map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, mtype, delta, value FROM metrics`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	gauges := make(map[string]float64)
	counters := make(map[string]int64)

	for rows.Next() {
		var (
			id    string
			mtype string
			delta sql.NullInt64
			value sql.NullFloat64
		)
		if err := rows.Scan(&id, &mtype, &delta, &value); err != nil {
			return nil, nil, err
		}

		switch mtype {
		case models.Gauge:
			gauges[id] = value.Float64
		case models.Counter:
			counters[id] = delta.Int64
		}
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return gauges, counters, nil
}
