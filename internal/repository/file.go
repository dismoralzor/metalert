package repository

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dismoralzor/metalert/internal/model"
)

// SaveToFile пишет метрики атомарно: во временный файл в ТОЙ ЖЕ директории,
// что и path (важно - та же файловая система, иначе os.Rename не атомарен),
// а затем переименовывает его поверх path. os.Rename - атомарная операция ОС:
// в любой момент path либо старая полная версия, либо новая полная - падение
// программы посреди записи никогда не оставит битый JSON на месте рабочего файла.
func SaveToFile(path string, metrics []models.Metrics) error {
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metrics: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file %s: %w", tmpPath, err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename %s to %s: %w", tmpPath, path, err)
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
