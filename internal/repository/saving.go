package repository

import (
	"go.uber.org/zap"

	"github.com/dismoralzor/metalert/internal/logger"
)

// SavingStorage пишет состояние в файл после каждой записи метрики - режим
// STORE_INTERVAL=0. Сделан декоратором, а не вызовами из хендлеров, чтобы
// синхронное сохранение не пришлось дублировать в каждом из них.
type SavingStorage struct {
	Storage
	path string
}

func NewSavingStorage(inner Storage, path string) *SavingStorage {
	return &SavingStorage{Storage: inner, path: path}
}

func (s *SavingStorage) UpdateGauge(name string, value float64) {
	s.Storage.UpdateGauge(name, value)
	s.save()
}

func (s *SavingStorage) UpdateCounter(name string, delta int64) {
	s.Storage.UpdateCounter(name, delta)
	s.save()
}

// Ошибку записи логируем, но запрос не валим: метрика уже принята в память.
func (s *SavingStorage) save() {
	if err := SaveToFile(s.path, Snapshot(s.Storage)); err != nil {
		logger.Log.Error("sync save", zap.Error(err))
	}
}
