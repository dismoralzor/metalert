// Package models описывает модель метрики, общую для сервера и агента:
// структуру Metrics (JSON-протокол) и константы её типов.
package models

// Типы метрик, которые понимает сервис.
const (
	Counter = "counter" // накопительный счётчик (значение в Delta)
	Gauge   = "gauge"   // мгновенное значение (в Value)
)

// Metrics - единица обмена метриками по JSON-протоколу.
//
// Модель намеренно плоская, без вложенности. Delta и Value - указатели, чтобы
// отличать заданное значение "0" от отсутствующего (nil не попадёт в JSON
// благодаря omitempty).
type Metrics struct {
	ID    string   `json:"id"`
	MType string   `json:"type"`
	Delta *int64   `json:"delta,omitempty"`
	Value *float64 `json:"value,omitempty"`
	Hash  string   `json:"hash,omitempty"`
}
