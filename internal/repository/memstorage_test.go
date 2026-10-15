package repository

import (
	"context"
	"testing"

	"github.com/dismoralzor/metalert/internal/model"
)

func TestMemStorage_UpdateGauge(t *testing.T) {
	m := NewMemStorage()

	m.UpdateGauge(context.Background(), "temperature", 10.0)
	m.UpdateGauge(context.Background(), "temperature", 20.0)

	if got := m.gauges["temperature"]; got != 20.0 {
		t.Errorf("gauges[temperature] = %v, want %v", got, 20.0)
	}
}

func TestMemStorage_UpdateCounter(t *testing.T) {
	m := NewMemStorage()

	m.UpdateCounter(context.Background(), "requests", 5)
	m.UpdateCounter(context.Background(), "requests", 3)

	if got := m.counters["requests"]; got != 8 {
		t.Errorf("counters[requests] = %v, want %v", got, 8)
	}
}

func TestMemStorage_UpdateBatch(t *testing.T) {
	m := NewMemStorage()

	value := 23.5
	deltaA, deltaB := int64(5), int64(3)
	metrics := []models.Metrics{
		{ID: "temperature", MType: models.Gauge, Value: &value},
		{ID: "requests", MType: models.Counter, Delta: &deltaA},
		{ID: "requests", MType: models.Counter, Delta: &deltaB},
	}

	if err := m.UpdateBatch(context.Background(), metrics); err != nil {
		t.Fatalf("UpdateBatch() error = %v", err)
	}

	if got := m.gauges["temperature"]; got != 23.5 {
		t.Errorf("gauges[temperature] = %v, want 23.5", got)
	}
	// Дубли counter в одном батче должны сложиться, а не перезаписаться.
	if got := m.counters["requests"]; got != 8 {
		t.Errorf("counters[requests] = %v, want 8", got)
	}
}

func TestMemStorage_Metrics(t *testing.T) {
	m := NewMemStorage()
	m.UpdateGauge(context.Background(), "temperature", 23.5)
	m.UpdateCounter(context.Background(), "requests", 10)

	got := m.Metrics(context.Background())
	if len(got) != 2 {
		t.Fatalf("Metrics() returned %d entries, want 2", len(got))
	}

	// Порядок из map рандомизирован, поэтому индексируем по имени перед проверкой.
	byName := make(map[string]models.Metrics, len(got))
	for _, metric := range got {
		byName[metric.ID] = metric
	}

	gauge := byName["temperature"]
	if gauge.MType != models.Gauge || gauge.Value == nil || *gauge.Value != 23.5 {
		t.Errorf("temperature = %+v, want {MType: gauge, Value: 23.5}", gauge)
	}
	counter := byName["requests"]
	if counter.MType != models.Counter || counter.Delta == nil || *counter.Delta != 10 {
		t.Errorf("requests = %+v, want {MType: counter, Delta: 10}", counter)
	}
}
