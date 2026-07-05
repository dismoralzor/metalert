package repository

import (
	"maps"
	"sync"
)

// Мьютекс нужен, т.к. конкурентная запись в map без синхронизации паникует.
// RWMutex, а не Mutex: чтения (GET /value, GET /) не должны блокировать друг друга.
type MemStorage struct {
	mu       sync.RWMutex
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

func (m *MemStorage) GetGauge(name string) (float64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.gauges[name]
	return value, ok
}

func (m *MemStorage) GetCounter(name string) (int64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.counters[name]
	return value, ok
}

// Возвращает копию, а не m.gauges напрямую - иначе вызывающий код читал бы
// map без мьютекса, и конкурентная запись рядом привела бы к panic.
func (m *MemStorage) Gauges() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]float64, len(m.gauges))
	maps.Copy(result, m.gauges)
	return result
}

func (m *MemStorage) Counters() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]int64, len(m.counters))
	maps.Copy(result, m.counters)
	return result
}