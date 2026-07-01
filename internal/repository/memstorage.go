package repository

import "sync"

// MemStorage — потокобезопасная реализация Storage поверх map в памяти.
// Мьютекс нужен, потому что http.Server обрабатывает запросы конкурентно,
// а конкурентная запись в map без синхронизации приводит к panic.
type MemStorage struct {
	mu       sync.Mutex
	gauges   map[string]float64
	counters map[string]int64
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (m *MemStorage) UpdateGauge(name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = value
}

// UpdateCounter прибавляет delta, а не заменяет значение.
func (m *MemStorage) UpdateCounter(name string, delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}