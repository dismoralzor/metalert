package repository

import (
	"context"

	"go.uber.org/zap"

	"github.com/dismoralzor/metalert/internal/logger"
	"github.com/dismoralzor/metalert/internal/model"
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

func (s *SavingStorage) UpdateGauge(ctx context.Context, name string, value float64) {
	s.Storage.UpdateGauge(ctx, name, value)
	s.save(ctx)
}

func (s *SavingStorage) UpdateCounter(ctx context.Context, name string, delta int64) {
	s.Storage.UpdateCounter(ctx, name, delta)
	s.save(ctx)
}

// UpdateBatch сохраняет файл ОДИН раз после всего батча, а не на каждую метрику -
// иначе батч из 30 метрик означал бы 30 перезаписей файла подряд.
func (s *SavingStorage) UpdateBatch(ctx context.Context, metrics []models.Metrics) error {
	if err := s.Storage.UpdateBatch(ctx, metrics); err != nil {
		return err
	}
	s.save(ctx)
	return nil
}

// Ошибку записи логируем, но запрос не валим: метрика уже принята в память.
func (s *SavingStorage) save(ctx context.Context) {
	if err := SaveToFile(s.path, Snapshot(ctx, s.Storage)); err != nil {
		logger.Log.Error("sync save", zap.Error(err))
	}
}
