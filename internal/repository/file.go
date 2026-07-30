package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

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

// Snapshot переводит DTO хранилища в JSON-модель. Строковые значения Metric
// разбираются обратно в числа; для float это безопасно, потому что MemStorage
// форматирует его с точностью -1 - кратчайшей записью, парсящейся в тот же float64.
func Snapshot(s Storage) ([]models.Metrics, error) {
	stored := s.Metrics()
	result := make([]models.Metrics, 0, len(stored))

	for _, metric := range stored {
		m := models.Metrics{ID: metric.Name, MType: metric.Type}

		switch metric.Type {
		case models.Gauge:
			value, err := strconv.ParseFloat(metric.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("parse gauge %s: %w", metric.Name, err)
			}
			m.Value = &value

		case models.Counter:
			delta, err := strconv.ParseInt(metric.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse counter %s: %w", metric.Name, err)
			}
			m.Delta = &delta

		default:
			return nil, fmt.Errorf("unknown metric type %q for %s", metric.Type, metric.Name)
		}

		result = append(result, m)
	}
	return result, nil
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
