package repository

import "github.com/dismoralzor/metalert/internal/model"

// Storage — контракт хранилища метрик, от которого зависит handler.
type Storage interface {
	UpdateGauge(name string, value float64)
	UpdateCounter(name string, delta int64)

	// bool отличает отсутствие метрики от значения 0.
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)

	// Value/Delta заполнены типизированно (см. models.Metrics) - без строковой
	// конвертации, форматирование в текст остаётся на вызывающей стороне.
	Metrics() []models.Metrics
}
