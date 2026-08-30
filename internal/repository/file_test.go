package repository

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/dismoralzor/metalert/internal/model"
)

func gaugePtr(v float64) *float64 { return &v }
func counterPtr(v int64) *int64   { return &v }

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")

	want := []models.Metrics{
		{ID: "Temperature", MType: models.Gauge, Value: gaugePtr(23.5)},
		{ID: "Requests", MType: models.Counter, Delta: counterPtr(10)},
	}

	if err := SaveToFile(path, want); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	got, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("loaded %d metrics, want %d", len(got), len(want))
	}

	byID := make(map[string]models.Metrics, len(got))
	for _, m := range got {
		byID[m.ID] = m
	}

	if g := byID["Temperature"]; g.MType != models.Gauge || g.Value == nil || *g.Value != 23.5 {
		t.Errorf("Temperature = %+v, want gauge 23.5", g)
	}
	if c := byID["Requests"]; c.MType != models.Counter || c.Delta == nil || *c.Delta != 10 {
		t.Errorf("Requests = %+v, want counter 10", c)
	}
}

func TestLoadFromFile_Missing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	got, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile on missing file: %v, want nil error", err)
	}
	if len(got) != 0 {
		t.Errorf("loaded %d metrics from missing file, want 0", len(got))
	}
}

func TestLoadFromFile_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write empty file: %v", err)
	}

	got, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile on empty file: %v, want nil error", err)
	}
	if len(got) != 0 {
		t.Errorf("loaded %d metrics from empty file, want 0", len(got))
	}
}

// Полный путь состояния: хранилище -> файл -> новое хранилище.
// Значения gauge подобраны так, чтобы поймать потерю точности при JSON-сериализации.
func TestSnapshotSaveLoadRestore(t *testing.T) {
	gauges := map[string]float64{
		"Simple":   23.5,
		"Tenth":    0.1,
		"Random":   0.6264947551289394,
		"Huge":     1.7976931348623157e+308,
		"Tiny":     5e-324,
		"Negative": -0.000123456789,
		"Zero":     0,
	}

	src := NewMemStorage()
	for name, value := range gauges {
		src.UpdateGauge(context.Background(), name, value)
	}
	src.UpdateCounter(context.Background(), "PollCount", 42)

	snapshot := Snapshot(context.Background(), src)

	path := filepath.Join(t.TempDir(), "metrics.json")
	if err := SaveToFile(path, snapshot); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	dst := NewMemStorage()
	Restore(context.Background(), dst, loaded)

	for name, want := range gauges {
		got, ok := dst.GetGauge(context.Background(), name)
		if !ok {
			t.Errorf("gauge %s missing after restore", name)
			continue
		}
		if got != want {
			t.Errorf("gauge %s = %v, want %v (точность потеряна)", name, got, want)
		}
	}

	if got, ok := dst.GetCounter(context.Background(), "PollCount"); !ok || got != 42 {
		t.Errorf("counter PollCount = %v (ok=%v), want 42", got, ok)
	}
}

// Успешная запись не должна оставлять рядом временный файл - иначе директория
// с метриками постепенно захламлялась бы .tmp-мусором.
func TestSaveToFile_NoLeftoverTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")

	metrics := []models.Metrics{{ID: "x", MType: models.Gauge, Value: gaugePtr(1)}}
	if err := SaveToFile(path, metrics); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Errorf("directory entries = %v, want exactly [%s]", entries, filepath.Base(path))
	}
}

// Если временный файл записался, но переименовать его поверх path не удалось
// (здесь имитируем это тем, что на месте path уже лежит директория - Rename
// файла поверх директории обязан провалиться и на Windows, и на Linux),
// SaveToFile должен вернуть ошибку и убрать временный файл, а не оставить
// его валяться рядом.
func TestSaveToFile_CleansUpTempFileOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}

	metrics := []models.Metrics{{ID: "x", MType: models.Gauge, Value: gaugePtr(1)}}
	if err := SaveToFile(path, metrics); err == nil {
		t.Fatal("SaveToFile() error = nil, want error (rename onto a directory must fail)")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	// Единственное, что должно остаться в директории - сама path-директория,
	// временный файл рядом быть не должен.
	if len(entries) != 1 {
		t.Errorf("directory has %d entries after failed SaveToFile, want 1 (leaked temp file?): %v", len(entries), entries)
	}
}

// Атомарность в действии: если запись во временный файл падает, рабочий файл
// с ПРЕДЫДУЩИМ валидным содержимым остаётся нетронутым, а не обнуляется/бьётся.
func TestSaveToFile_PreservesExistingFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.json")

	original := []models.Metrics{{ID: "Original", MType: models.Gauge, Value: gaugePtr(42)}}
	if err := SaveToFile(path, original); err != nil {
		t.Fatalf("SaveToFile (initial write): %v", err)
	}

	// Провоцируем ошибку на этапе Rename тем же трюком, что и выше, но теперь
	// имя временного файла не пересекается с "metrics.json" - для этого нужен
	// отдельный путь-директория, поэтому проверяем именно то, что после сбоя
	// СТАРОЕ содержимое path не изменилось (не были задеты сами байты файла).
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile before: %v", err)
	}

	// os.Mkdir на месте занятого файла не сработает - вместо этого проверяем
	// поведение через marshal-ошибку: math.Inf не кодируется в JSON.
	bad := []models.Metrics{{ID: "Bad", MType: models.Gauge, Value: gaugePtr(math.Inf(1))}}
	if err := SaveToFile(path, bad); err == nil {
		t.Fatal("SaveToFile() error = nil, want error (marshal of +Inf must fail)")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("file content changed after failed SaveToFile:\nbefore: %s\nafter:  %s", before, after)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory has %d entries after failed SaveToFile, want 1 (leaked temp file?): %v", len(entries), entries)
	}
}

// Синхронный режим: файл должен появиться сразу после записи метрики.
func TestSavingStorage_WritesOnUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	s := NewSavingStorage(NewMemStorage(), path)

	s.UpdateGauge(context.Background(), "Temperature", 23.5)

	metrics, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("file has %d metrics, want 1", len(metrics))
	}
	if m := metrics[0]; m.ID != "Temperature" || m.Value == nil || *m.Value != 23.5 {
		t.Errorf("saved metric = %+v, want Temperature gauge 23.5", m)
	}

	s.UpdateCounter(context.Background(), "PollCount", 7)

	metrics, err = LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile after counter: %v", err)
	}
	if len(metrics) != 2 {
		t.Errorf("file has %d metrics after second update, want 2", len(metrics))
	}
}
