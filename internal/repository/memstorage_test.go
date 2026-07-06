package repository

import (
	"testing"

	"github.com/dismoralzor/metalert/internal/model"
)

func TestMemStorage_UpdateGauge(t *testing.T) {
	m := NewMemStorage()

	m.UpdateGauge("temperature", 10.0)
	m.UpdateGauge("temperature", 20.0)

	if got := m.gauges["temperature"]; got != 20.0 {
		t.Errorf("gauges[temperature] = %v, want %v", got, 20.0)
	}
}

func TestMemStorage_UpdateCounter(t *testing.T) {
	m := NewMemStorage()

	m.UpdateCounter("requests", 5)
	m.UpdateCounter("requests", 3)

	if got := m.counters["requests"]; got != 8 {
		t.Errorf("counters[requests] = %v, want %v", got, 8)
	}
}

func TestMemStorage_Metrics(t *testing.T) {
	m := NewMemStorage()
	m.UpdateGauge("temperature", 23.5)
	m.UpdateCounter("requests", 10)

	got := m.Metrics()
	if len(got) != 2 {
		t.Fatalf("Metrics() returned %d entries, want 2", len(got))
	}

	// Порядок из map рандомизирован, поэтому индексируем по имени перед проверкой.
	byName := make(map[string]Metric, len(got))
	for _, metric := range got {
		byName[metric.Name] = metric
	}

	if gauge := byName["temperature"]; gauge.Type != models.Gauge || gauge.Value != "23.5" {
		t.Errorf("temperature = %+v, want {Type: gauge, Value: 23.5}", gauge)
	}
	if counter := byName["requests"]; counter.Type != models.Counter || counter.Value != "10" {
		t.Errorf("requests = %+v, want {Type: counter, Value: 10}", counter)
	}
}
