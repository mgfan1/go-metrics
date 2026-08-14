package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"sort"
	"syscall"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
	"github.com/mgfan1/go-metrics/internal/retry"
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

func NewPGStorage(ctx context.Context, db *sql.DB, log *zap.Logger) (*PGStorage, error) {
	s := &PGStorage{db: db, log: log}

	if err := s.retry(ctx, func() error { return applyMigrations(ctx, db) }); err != nil {
		return nil, err
	}
	log.Info("схема метрик готова")

	return s, nil
}

func retriablePG(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return retriableCode(pgErr.Code)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, driver.ErrBadConn)
}

func retriableCode(code string) bool {
	if pgerrcode.IsConnectionException(code) || pgerrcode.IsTransactionRollback(code) {
		return true
	}

	switch code {
	case pgerrcode.CannotConnectNow, pgerrcode.AdminShutdown, pgerrcode.CrashShutdown:
		return true
	default:
		return false
	}
}

func (s *PGStorage) retry(ctx context.Context, op func() error) error {
	return retry.Do(ctx, s.log, retriablePG, op)
}

func applyMigrations(ctx context.Context, db *sql.DB) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("не прочитал миграции: %w", err)
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("не получил соединение для миграций: %w", err)
	}
	defer conn.Close()

	drv, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
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
	return s.retry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, upsertGauge, name, value)
		return err
	})
}

func (s *PGStorage) AddCounter(ctx context.Context, name string, delta int64) error {
	return s.retry(ctx, func() error {
		_, err := s.db.ExecContext(ctx, upsertCounter, name, delta)
		return err
	})
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
	return s.retry(ctx, func() error { return s.updateBatch(ctx, batch) })
}

func (s *PGStorage) updateBatch(ctx context.Context, batch []models.Metrics) error {
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

	err := s.retry(ctx, func() error {
		row := s.db.QueryRowContext(ctx, `SELECT value FROM metrics WHERE id = $1 AND mtype = 'gauge'`, name)
		return row.Scan(&value)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}

	return value.Float64, nil
}

func (s *PGStorage) Counter(ctx context.Context, name string) (int64, error) {
	var delta sql.NullInt64

	err := s.retry(ctx, func() error {
		row := s.db.QueryRowContext(ctx, `SELECT delta FROM metrics WHERE id = $1 AND mtype = 'counter'`, name)
		return row.Scan(&delta)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, err
	}

	return delta.Int64, nil
}

func (s *PGStorage) Snapshot(ctx context.Context) (map[string]float64, map[string]int64, error) {
	gauges := make(map[string]float64)
	counters := make(map[string]int64)

	err := s.retry(ctx, func() error {
		clear(gauges)
		clear(counters)
		return s.snapshot(ctx, gauges, counters)
	})
	if err != nil {
		return nil, nil, err
	}

	return gauges, counters, nil
}

func (s *PGStorage) snapshot(ctx context.Context, gauges map[string]float64, counters map[string]int64) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, mtype, delta, value FROM metrics`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id    string
			mtype string
			delta sql.NullInt64
			value sql.NullFloat64
		)
		if err := rows.Scan(&id, &mtype, &delta, &value); err != nil {
			return err
		}

		switch mtype {
		case models.Gauge:
			gauges[id] = value.Float64
		case models.Counter:
			counters[id] = delta.Int64
		}
	}

	return rows.Err()
}
