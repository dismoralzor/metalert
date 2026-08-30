package repository

import (
	"context"

	"github.com/dismoralzor/metalert/internal/model"
)

// Storage — контракт хранилища метрик, от которого зависит handler.
//
// Все методы принимают ctx первым параметром - DBStorage пробрасывает его
// в ExecContext/QueryContext и в retry.Do (отмена запроса клиентом или таймаут
// должны прерывать реальный SQL-запрос, а не только "текущий шаг" метода).
// MemStorage ctx игнорирует - работа с map ничего не ждёт и не может зависнуть.
// Возвращаемые типы старых методов НЕ меняются: error они по-прежнему не
// возвращают (ошибки БД в них логируются на месте, см. DBStorage).
type Storage interface {
	UpdateGauge(ctx context.Context, name string, value float64)
	UpdateCounter(ctx context.Context, name string, delta int64)

	// bool отличает отсутствие метрики от значения 0.
	GetGauge(ctx context.Context, name string) (float64, bool)
	GetCounter(ctx context.Context, name string) (int64, bool)

	// Value/Delta заполнены типизированно (см. models.Metrics) - без строковой
	// конвертации, форматирование в текст остаётся на вызывающей стороне.
	Metrics(ctx context.Context) []models.Metrics

	// UpdateBatch, в отличие от методов выше, возвращает error: у DBStorage
	// это одна транзакция на весь батч, а транзакция может упасть.
	UpdateBatch(ctx context.Context, metrics []models.Metrics) error
}
