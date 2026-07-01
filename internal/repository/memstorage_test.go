package repository

import "testing"

func TestMemStorage_UpdateGauge(t *testing.T) {
	m := NewMemStorage()

	m.UpdateGauge("temperature", 10.0)
	m.UpdateGauge("temperature", 20.0)

	// gauge - это "текущее значение", а не накопление,
	// поэтому второй вызов должен полностью вытеснить первый.
	if got := m.gauges["temperature"]; got != 20.0 {
		t.Errorf("gauges[temperature] = %v, want %v", got, 20.0)
	}
}

func TestMemStorage_UpdateCounter(t *testing.T) {
	m := NewMemStorage()

	m.UpdateCounter("requests", 5)
	m.UpdateCounter("requests", 3)

	// counter, наоборот, накапливается: второй вызов прибавляется к первому.
	if got := m.counters["requests"]; got != 8 {
		t.Errorf("counters[requests] = %v, want %v", got, 8)
	}
}
