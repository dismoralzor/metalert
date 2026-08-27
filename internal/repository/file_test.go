package repository

import (
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
		src.UpdateGauge(name, value)
	}
	src.UpdateCounter("PollCount", 42)

	snapshot := Snapshot(src)

	path := filepath.Join(t.TempDir(), "metrics.json")
	if err := SaveToFile(path, snapshot); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}

	dst := NewMemStorage()
	Restore(dst, loaded)

	for name, want := range gauges {
		got, ok := dst.GetGauge(name)
		if !ok {
			t.Errorf("gauge %s missing after restore", name)
			continue
		}
		if got != want {
			t.Errorf("gauge %s = %v, want %v (точность потеряна)", name, got, want)
		}
	}

	if got, ok := dst.GetCounter("PollCount"); !ok || got != 42 {
		t.Errorf("counter PollCount = %v (ok=%v), want 42", got, ok)
	}
}

// Синхронный режим: файл должен появиться сразу после записи метрики.
func TestSavingStorage_WritesOnUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.json")
	s := NewSavingStorage(NewMemStorage(), path)

	s.UpdateGauge("Temperature", 23.5)

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

	s.UpdateCounter("PollCount", 7)

	metrics, err = LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile after counter: %v", err)
	}
	if len(metrics) != 2 {
		t.Errorf("file has %d metrics after second update, want 2", len(metrics))
	}
}
