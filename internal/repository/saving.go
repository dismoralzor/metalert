package repository

import (
	"context"

	"go.uber.org/zap"

	models "github.com/dismoralzor/metalert/internal/model"
)

// SavingStorage пишет состояние в файл после каждой записи метрики - режим
// STORE_INTERVAL=0. Сделан декоратором, а не вызовами из хендлеров, чтобы
// синхронное сохранение не пришлось дублировать в каждом из них. Внутреннее
// хранилище - именованное приватное поле, а не встраивание: иначе в публичный
// API декоратора улетали бы все методы Storage.
type SavingStorage struct {
	inner Storage
	path  string
	log   *zap.Logger
}

// NewSavingStorage оборачивает inner так, чтобы после каждой записи метрики
// состояние синхронно сохранялось в файл path (режим STORE_INTERVAL=0).
// log используется для сообщений об ошибках записи.
func NewSavingStorage(inner Storage, path string, log *zap.Logger) *SavingStorage {
	return &SavingStorage{inner: inner, path: path, log: log}
}

// UpdateGauge записывает метрику во внутреннее хранилище и сохраняет файл.
func (s *SavingStorage) UpdateGauge(ctx context.Context, name string, value float64) {
	s.inner.UpdateGauge(ctx, name, value)
	s.save(ctx)
}

// UpdateCounter прибавляет delta во внутреннем хранилище и сохраняет файл.
func (s *SavingStorage) UpdateCounter(ctx context.Context, name string, delta int64) {
	s.inner.UpdateCounter(ctx, name, delta)
	s.save(ctx)
}

// UpdateBatch сохраняет файл ОДИН раз после всего батча, а не на каждую метрику -
// иначе батч из 30 метрик означал бы 30 перезаписей файла подряд.
func (s *SavingStorage) UpdateBatch(ctx context.Context, metrics []models.Metrics) error {
	if err := s.inner.UpdateBatch(ctx, metrics); err != nil {
		return err
	}
	s.save(ctx)
	return nil
}

// GetGauge возвращает gauge из внутреннего хранилища.
func (s *SavingStorage) GetGauge(ctx context.Context, name string) (float64, bool) {
	return s.inner.GetGauge(ctx, name)
}

// GetCounter возвращает counter из внутреннего хранилища.
func (s *SavingStorage) GetCounter(ctx context.Context, name string) (int64, bool) {
	return s.inner.GetCounter(ctx, name)
}

// Metrics возвращает снимок метрик из внутреннего хранилища.
func (s *SavingStorage) Metrics(ctx context.Context) []models.Metrics {
	return s.inner.Metrics(ctx)
}

// Ошибку записи логируем, но запрос не валим: метрика уже принята в память.
func (s *SavingStorage) save(ctx context.Context) {
	if err := SaveToFile(s.path, Snapshot(ctx, s.inner)); err != nil {
		s.log.Error("sync save", zap.Error(err))
	}
}
