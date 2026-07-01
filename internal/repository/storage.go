package repository

// Storage — контракт хранилища метрик, от которого зависит handler.
type Storage interface {
	UpdateGauge(name string, value float64)
	UpdateCounter(name string, delta int64)
}