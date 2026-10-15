package repository

import (
	"context"
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

// ctx игнорируется во всех методах MemStorage: работа с map в памяти не делает
// внешних вызовов и не может зависнуть - отменять тут нечего.

func (m *MemStorage) UpdateGauge(_ context.Context, name string, value float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = value
}

// UpdateCounter прибавляет delta, а не заменяет значение.
func (m *MemStorage) UpdateCounter(_ context.Context, name string, delta int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name] += delta
}

func (m *MemStorage) GetGauge(_ context.Context, name string) (float64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.gauges[name]
	return value, ok
}

func (m *MemStorage) GetCounter(_ context.Context, name string) (int64, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.counters[name]
	return value, ok
}

// UpdateBatch применяет весь батч под ОДНИМ Lock - без него запрос из 30 метрик
// дёргал бы мьютекс 30 раз, каждый раз рискуя пропустить вперёд другого writer'а
// и получить metrics-снапшот из вперемешку старых и новых значений.
// Логика UpdateGauge/UpdateCounter продублирована инлайном, а не вызвана напрямую:
// RWMutex не реентерабельный, повторный Lock изнутри уже захваченного - дедлок.
func (m *MemStorage) UpdateBatch(_ context.Context, metrics []models.Metrics) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, metric := range metrics {
		switch metric.MType {
		case models.Gauge:
			if metric.Value != nil {
				m.gauges[metric.ID] = *metric.Value
			}
		case models.Counter:
			if metric.Delta != nil {
				m.counters[metric.ID] += *metric.Delta
			}
		}
	}

	return nil
}

// Metrics строит DTO под мьютексом, а не отдаёт вызывающему коду доступ
// к m.gauges/m.counters напрямую - иначе чтение без мьютекса и конкурентная
// запись рядом привели бы к panic.
func (m *MemStorage) Metrics(_ context.Context) []models.Metrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]models.Metrics, 0, len(m.gauges)+len(m.counters))
	for name, value := range m.gauges {
		v := value
		result = append(result, models.Metrics{ID: name, MType: models.Gauge, Value: &v})
	}
	for name, delta := range m.counters {
		d := delta
		result = append(result, models.Metrics{ID: name, MType: models.Counter, Delta: &d})
	}
	return result
}
