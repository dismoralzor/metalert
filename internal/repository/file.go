package repository

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dismoralzor/metalert/internal/model"
)

func SaveToFile(path string, metrics []models.Metrics) error {
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// LoadFromFile возвращает пустой срез, если файла нет: первый запуск - это не ошибка.
func LoadFromFile(path string) ([]models.Metrics, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	// Пустой файл json.Unmarshal считает ошибкой, хотя для нас это тоже "нечего грузить".
	if len(data) == 0 {
		return nil, nil
	}

	var metrics []models.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return metrics, nil
}

// Snapshot возвращает текущее состояние хранилища в формате JSON-модели.
func Snapshot(s Storage) []models.Metrics {
	return s.Metrics()
}

// Restore заливает метрики в хранилище. Рассчитан на однократный вызов при старте:
// UpdateCounter прибавляет, поэтому повторный вызов удвоил бы счётчики.
func Restore(s Storage, metrics []models.Metrics) {
	for _, m := range metrics {
		switch m.MType {
		case models.Gauge:
			if m.Value != nil {
				s.UpdateGauge(m.ID, *m.Value)
			}
		case models.Counter:
			if m.Delta != nil {
				s.UpdateCounter(m.ID, *m.Delta)
			}
		}
	}
}
