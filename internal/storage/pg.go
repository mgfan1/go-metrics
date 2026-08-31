package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"sort"
	"strings"
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
	db      *sql.DB
	retrier *retry.Retrier
}

func NewPGStorage(ctx context.Context, db *sql.DB, log *zap.Logger) (*PGStorage, error) {
	s := &PGStorage{db: db, retrier: retry.New(log)}

	if err := s.retry(ctx, func() error { return applyMigrations(ctx, db, migrations.FS, ".") }); err != nil {
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
	case pgerrcode.CannotConnectNow, pgerrcode.AdminShutdown, pgerrcode.CrashShutdown, pgerrcode.TooManyConnections:
		return true
	default:
		return false
	}
}

func (s *PGStorage) retry(ctx context.Context, op func() error) error {
	return s.retrier.Do(ctx, retriablePG, op)
}

func applyMigrations(ctx context.Context, db *sql.DB, src fs.FS, path string) error {
	source, err := iofs.New(src, path)
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

	m, err := migrate.NewWithInstance("iofs", source, "postgres", drv)
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

func upsertBatch(mtype, column, update string, args []any) string {
	var b strings.Builder
	b.WriteString("INSERT INTO metrics (id, mtype, " + column + ") VALUES ")

	for i := 0; i < len(args); i += 2 {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "($%d, '%s', $%d)", i+1, mtype, i+2)
	}

	b.WriteString(" ON CONFLICT (id, mtype) DO UPDATE SET " + update)

	return b.String()
}

func (s *PGStorage) updateBatch(ctx context.Context, batch []models.Metrics) error {
	gauges := make([]any, 0, len(batch)*2)
	counters := make([]any, 0, len(batch)*2)

	for _, m := range batch {
		switch m.MType {
		case models.Gauge:
			gauges = append(gauges, m.ID, *m.Value)
		case models.Counter:
			counters = append(counters, m.ID, *m.Delta)
		}
	}

	if len(gauges) == 0 && len(counters) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("не начал транзакцию: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if len(gauges) > 0 {
		query := upsertBatch(models.Gauge, "value", "value = EXCLUDED.value", gauges)
		if _, err := tx.ExecContext(ctx, query, gauges...); err != nil {
			return err
		}
	}

	if len(counters) > 0 {
		query := upsertBatch(models.Counter, "delta", "delta = metrics.delta + EXCLUDED.delta", counters)
		if _, err := tx.ExecContext(ctx, query, counters...); err != nil {
			return err
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
