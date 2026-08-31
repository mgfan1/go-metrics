package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.uber.org/zap"

	models "github.com/mgfan1/go-metrics/internal/model"
)

func newFileStore(t *testing.T, repo Repository, path string, restore bool) *FileStore {
	t.Helper()

	files, err := NewFileStore(repo, path, restore, zap.NewNop())
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return files
}

func TestSaveThenLoad(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "metrics.json")

	src := NewMemStorage()
	src.UpdateGauge(ctx, "Alloc", 42.5)
	src.AddCounter(ctx, "PollCount", 7)

	if err := newFileStore(t, src, path, false).save(ctx); err != nil {
		t.Fatalf("save: %v", err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, err := dst.Gauge(ctx, "Alloc"); err != nil || v != 42.5 {
		t.Errorf("gauge = %v, %v; хотел 42.5, nil", v, err)
	}
	if v, err := dst.Counter(ctx, "PollCount"); err != nil || v != 7 {
		t.Errorf("counter = %v, %v; хотел 7, nil", v, err)
	}
}

func TestRestoreDisabled(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "metrics.json")

	src := NewMemStorage()
	src.UpdateGauge(ctx, "Alloc", 42.5)
	if err := newFileStore(t, src, path, false).save(ctx); err != nil {
		t.Fatal(err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, false)

	if _, err := dst.Gauge(ctx, "Alloc"); !errors.Is(err, ErrNotFound) {
		t.Error("при restore=false метрики загружаться не должны")
	}
}

func TestLoadMissingFile(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "нет-такого.json")

	store := NewMemStorage()
	if _, err := NewFileStore(store, path, true, zap.NewNop()); err != nil {
		t.Errorf("отсутствие файла не должно быть ошибкой: %v", err)
	}

	gauges, counters, err := store.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(gauges) != 0 || len(counters) != 0 {
		t.Error("хранилище должно остаться пустым")
	}
}

func TestLoadCorruptedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	if err := os.WriteFile(path, []byte("не json"), 0o666); err != nil {
		t.Fatal(err)
	}

	if _, err := NewFileStore(NewMemStorage(), path, true, zap.NewNop()); err == nil {
		t.Error("для битого файла ждал ошибку")
	}
}

func TestSaveOverwrites(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "metrics.json")

	store := NewMemStorage()
	files := newFileStore(t, store, path, false)

	store.UpdateGauge(ctx, "Alloc", 1)
	if err := files.save(ctx); err != nil {
		t.Fatal(err)
	}

	store.UpdateGauge(ctx, "Alloc", 2)
	if err := files.save(ctx); err != nil {
		t.Fatal(err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, _ := dst.Gauge(ctx, "Alloc"); v != 2 {
		t.Errorf("gauge = %v, хотел 2: старое значение не перезаписано", v)
	}
}

func TestCloseSavesMetrics(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "metrics.json")

	store := NewMemStorage()
	store.UpdateGauge(ctx, "Alloc", 8.25)

	if err := newFileStore(t, store, path, false).Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, err := dst.Gauge(ctx, "Alloc"); err != nil || v != 8.25 {
		t.Errorf("gauge = %v, %v; Close не сохранил метрики", v, err)
	}
}

func TestSyncRepositorySavesOnWrite(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "metrics.json")

	repo := newFileStore(t, NewMemStorage(), path, false).SyncRepository()
	if err := repo.UpdateGauge(ctx, "Alloc", 3.5); err != nil {
		t.Fatalf("UpdateGauge: %v", err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, err := dst.Gauge(ctx, "Alloc"); err != nil || v != 3.5 {
		t.Errorf("gauge = %v, %v; синхронная запись не сработала", v, err)
	}
}

func TestSyncRepositorySavesBatch(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "metrics.json")

	repo := newFileStore(t, NewMemStorage(), path, false).SyncRepository()
	batch := []models.Metrics{counter("PollCount", 5), gauge("Alloc", 1.5), counter("PollCount", 3)}
	if err := repo.UpdateBatch(ctx, batch); err != nil {
		t.Fatalf("UpdateBatch: %v", err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, err := dst.Gauge(ctx, "Alloc"); err != nil || v != 1.5 {
		t.Errorf("gauge = %v, %v; батч не сохранён на диск", v, err)
	}
	if v, err := dst.Counter(ctx, "PollCount"); err != nil || v != 8 {
		t.Errorf("counter = %v, %v; хотел 8, nil", v, err)
	}
}

func TestConcurrentSaveKeepsFileValid(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "metrics.json")

	repo := newFileStore(t, NewMemStorage(), path, false).SyncRepository()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				repo.UpdateGauge(ctx, "Alloc", float64(n*1000+j))
			}
		}(i)
	}
	wg.Wait()

	dst := NewMemStorage()
	if _, err := NewFileStore(dst, path, true, zap.NewNop()); err != nil {
		t.Fatalf("файл повреждён параллельной записью: %v", err)
	}
	if _, err := dst.Gauge(ctx, "Alloc"); err != nil {
		t.Errorf("после параллельных записей метрика потерялась: %v", err)
	}
}

func TestEmptyPathDoesNothing(t *testing.T) {
	store := NewMemStorage()
	store.UpdateGauge(t.Context(), "Alloc", 1)

	files := newFileStore(t, store, "", true)
	if err := files.Close(); err != nil {
		t.Errorf("Close с пустым путём: %v", err)
	}
}
