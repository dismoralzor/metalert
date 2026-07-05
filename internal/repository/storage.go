package repository

// Storage — контракт хранилища метрик, от которого зависит handler.
type Storage interface {
	UpdateGauge(name string, value float64)
	UpdateCounter(name string, delta int64)

	// bool отличает отсутствие метрики от значения 0.
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)

	Gauges() map[string]float64
	Counters() map[string]int64
}