package repository

// Metric - DTO для чтения списком: скрывает от вызывающего кода, что внутри
// MemStorage gauge и counter лежат в двух разных map с разными типами значений.
type Metric struct {
	Name  string
	Type  string // "gauge" или "counter" (см. models.Gauge / models.Counter)
	Value string
}

// Storage — контракт хранилища метрик, от которого зависит handler.
type Storage interface {
	UpdateGauge(name string, value float64)
	UpdateCounter(name string, delta int64)

	// bool отличает отсутствие метрики от значения 0.
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)

	Metrics() []Metric
}