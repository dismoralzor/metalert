package repository

import (
	"strconv"
	"sync"

	"github.com/dismoralzor/metalert/internal/model"
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

// Metrics строит DTO под мьютексом, а не отдаёт вызывающему коду доступ
// к m.gauges/m.counters напрямую - иначе чтение без мьютекса и конкурентная
// запись рядом привели бы к panic.
func (m *MemStorage) Metrics() []Metric {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Metric, 0, len(m.gauges)+len(m.counters))
	for name, value := range m.gauges {
		result = append(result, Metric{
			Name:  name,
			Type:  models.Gauge,
			Value: strconv.FormatFloat(value, 'f', -1, 64),
		})
	}
	for name, value := range m.counters {
		result = append(result, Metric{
			Name:  name,
			Type:  models.Counter,
			Value: strconv.FormatInt(value, 10),
		})
	}
	return result
}