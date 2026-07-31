package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.uber.org/zap"
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
	path := filepath.Join(t.TempDir(), "metrics.json")

	src := NewMemStorage()
	src.UpdateGauge("Alloc", 42.5)
	src.AddCounter("PollCount", 7)

	if err := newFileStore(t, src, path, false).save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, ok := dst.Gauge("Alloc"); !ok || v != 42.5 {
		t.Errorf("gauge = %v, %v; хотел 42.5, true", v, ok)
	}
	if v, ok := dst.Counter("PollCount"); !ok || v != 7 {
		t.Errorf("counter = %v, %v; хотел 7, true", v, ok)
	}
}

func TestRestoreDisabled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	src := NewMemStorage()
	src.UpdateGauge("Alloc", 42.5)
	if err := newFileStore(t, src, path, false).save(); err != nil {
		t.Fatal(err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, false)

	if _, ok := dst.Gauge("Alloc"); ok {
		t.Error("при restore=false метрики загружаться не должны")
	}
}

func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "нет-такого.json")

	store := NewMemStorage()
	if _, err := NewFileStore(store, path, true, zap.NewNop()); err != nil {
		t.Errorf("отсутствие файла не должно быть ошибкой: %v", err)
	}

	gauges, counters := store.Snapshot()
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
	path := filepath.Join(t.TempDir(), "metrics.json")

	store := NewMemStorage()
	files := newFileStore(t, store, path, false)

	store.UpdateGauge("Alloc", 1)
	if err := files.save(); err != nil {
		t.Fatal(err)
	}

	store.UpdateGauge("Alloc", 2)
	if err := files.save(); err != nil {
		t.Fatal(err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, _ := dst.Gauge("Alloc"); v != 2 {
		t.Errorf("gauge = %v, хотел 2: старое значение не перезаписано", v)
	}
}

func TestCloseSavesMetrics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	store := NewMemStorage()
	store.UpdateGauge("Alloc", 8.25)

	if err := newFileStore(t, store, path, false).Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, ok := dst.Gauge("Alloc"); !ok || v != 8.25 {
		t.Errorf("gauge = %v, %v; Close не сохранил метрики", v, ok)
	}
}

func TestSyncRepositorySavesOnWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	repo := newFileStore(t, NewMemStorage(), path, false).SyncRepository()
	repo.UpdateGauge("Alloc", 3.5)

	dst := NewMemStorage()
	newFileStore(t, dst, path, true)

	if v, ok := dst.Gauge("Alloc"); !ok || v != 3.5 {
		t.Errorf("gauge = %v, %v; синхронная запись не сработала", v, ok)
	}
}

func TestConcurrentSaveKeepsFileValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	repo := newFileStore(t, NewMemStorage(), path, false).SyncRepository()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				repo.UpdateGauge("Alloc", float64(n*1000+j))
			}
		}(i)
	}
	wg.Wait()

	dst := NewMemStorage()
	if _, err := NewFileStore(dst, path, true, zap.NewNop()); err != nil {
		t.Fatalf("файл повреждён параллельной записью: %v", err)
	}
	if _, ok := dst.Gauge("Alloc"); !ok {
		t.Error("после параллельных записей метрика потерялась")
	}
}

func TestEmptyPathDoesNothing(t *testing.T) {
	store := NewMemStorage()
	store.UpdateGauge("Alloc", 1)

	files := newFileStore(t, store, "", true)
	if err := files.Close(); err != nil {
		t.Errorf("Close с пустым путём: %v", err)
	}
}
