// Package models описывает модель метрики, общую для сервера и агента:
// структуру Metrics (JSON-протокол), константы её типов и правила валидации,
// единые для всех транспортов (HTTP и gRPC).
package models

import (
	"errors"
	"fmt"
)

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

// Validate проверяет, что метрика заполнена по правилам своего типа: у gauge
// должно быть Value, у counter - Delta, имя обязательно всегда.
func (m Metrics) Validate() error {
	if m.ID == "" {
		return errors.New("metric id is required")
	}

	switch m.MType {
	case Gauge:
		if m.Value == nil {
			return fmt.Errorf("value is required for gauge %q", m.ID)
		}
	case Counter:
		if m.Delta == nil {
			return fmt.Errorf("delta is required for counter %q", m.ID)
		}
	default:
		return fmt.Errorf("unknown metric type %q", m.MType)
	}

	return nil
}

// ValidateBatch проверяет весь батч ДО записи: частично применённый батч
// (первые N метрик прошли, N+1-я невалидна) хуже, чем явный отказ целиком.
func ValidateBatch(metrics []Metrics) error {
	if len(metrics) == 0 {
		return errors.New("empty batch")
	}
	for _, m := range metrics {
		if err := m.Validate(); err != nil {
			return err
		}
	}
	return nil
}
