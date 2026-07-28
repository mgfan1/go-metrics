package storage

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestSaveThenLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	src := NewMemStorage()
	src.UpdateGauge("Alloc", 42.5)
	src.AddCounter("PollCount", 7)

	if err := NewFileStore(src, path).Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dst := NewMemStorage()
	if err := NewFileStore(dst, path).Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v, ok := dst.Gauge("Alloc"); !ok || v != 42.5 {
		t.Errorf("gauge = %v, %v; хотел 42.5, true", v, ok)
	}
	if v, ok := dst.Counter("PollCount"); !ok || v != 7 {
		t.Errorf("counter = %v, %v; хотел 7, true", v, ok)
	}
}

func TestLoadMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "нет-такого.json")

	store := NewMemStorage()
	if err := NewFileStore(store, path).Load(); err != nil {
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

	if err := NewFileStore(NewMemStorage(), path).Load(); err == nil {
		t.Error("для битого файла ждал ошибку")
	}
}

func TestSaveOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	store := NewMemStorage()
	files := NewFileStore(store, path)

	store.UpdateGauge("Alloc", 1)
	if err := files.Save(); err != nil {
		t.Fatal(err)
	}

	store.UpdateGauge("Alloc", 2)
	if err := files.Save(); err != nil {
		t.Fatal(err)
	}

	dst := NewMemStorage()
	if err := NewFileStore(dst, path).Load(); err != nil {
		t.Fatal(err)
	}

	if v, _ := dst.Gauge("Alloc"); v != 2 {
		t.Errorf("gauge = %v, хотел 2: старое значение не перезаписано", v)
	}
}

func TestSyncRepositorySavesOnWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	files := NewFileStore(NewMemStorage(), path)
	repo := files.SyncRepository()

	repo.UpdateGauge("Alloc", 3.5)

	dst := NewMemStorage()
	if err := NewFileStore(dst, path).Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if v, ok := dst.Gauge("Alloc"); !ok || v != 3.5 {
		t.Errorf("gauge = %v, %v; синхронная запись не сработала", v, ok)
	}
}

func TestConcurrentSaveKeepsFileValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	store := NewMemStorage()
	files := NewFileStore(store, path)
	repo := files.SyncRepository()

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
	if err := NewFileStore(dst, path).Load(); err != nil {
		t.Fatalf("файл повреждён параллельной записью: %v", err)
	}
	if _, ok := dst.Gauge("Alloc"); !ok {
		t.Error("после параллельных записей метрика потерялась")
	}
}

func TestEmptyPathDoesNothing(t *testing.T) {
	store := NewMemStorage()
	store.UpdateGauge("Alloc", 1)

	files := NewFileStore(store, "")
	if err := files.Save(); err != nil {
		t.Errorf("Save с пустым путём: %v", err)
	}
	if err := files.Load(); err != nil {
		t.Errorf("Load с пустым путём: %v", err)
	}
}
