package repository

import (
	"context"
	"database/sql"
	"errors"

	"go.uber.org/zap"

	"github.com/dismoralzor/metalert/internal/logger"
	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/retry"
)

// DBStorage реализует Storage поверх PostgreSQL: одна таблица metrics,
// значение gauge/counter лежит в колонке value/delta в зависимости от type.
type DBStorage struct {
	db *sql.DB
}

func NewDBStorage(db *sql.DB) *DBStorage {
	return &DBStorage{db: db}
}

// Ошибку логируем: интерфейс Storage не позволяет вернуть её вызывающей стороне.
// retry.Do оборачивает Exec целиком - при обрыве соединения (Class 08) повторяем
// тот же запрос ещё до 3 раз с паузами 1s/3s/5s. ctx приходит от вызывающей
// стороны (у хендлеров - r.Context()) и прокидывается и в retry.Do, и в
// ExecContext - отмена запроса клиентом обязана прервать реальный SQL-запрос,
// а не только "текущую попытку" retry.
func (s *DBStorage) UpdateGauge(ctx context.Context, name string, value float64) {
	err := retry.Do(ctx, func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO metrics (id, type, value) VALUES ($1, 'gauge', $2)
			 ON CONFLICT (id) DO UPDATE SET value = $2, type = 'gauge'`,
			name, value,
		)
		return err
	}, retry.IsRetriablePG)
	if err != nil {
		logger.Log.Error("update gauge", zap.String("name", name), zap.Error(err))
	}
}

// UpdateCounter прибавляет delta, а не заменяет значение - как и MemStorage.
func (s *DBStorage) UpdateCounter(ctx context.Context, name string, delta int64) {
	err := retry.Do(ctx, func() error {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO metrics (id, type, delta) VALUES ($1, 'counter', $2)
			 ON CONFLICT (id) DO UPDATE SET delta = metrics.delta + $2, type = 'counter'`,
			name, delta,
		)
		return err
	}, retry.IsRetriablePG)
	if err != nil {
		logger.Log.Error("update counter", zap.String("name", name), zap.Error(err))
	}
}

func (s *DBStorage) GetGauge(ctx context.Context, name string) (float64, bool) {
	var value sql.NullFloat64
	err := retry.Do(ctx, func() error {
		return s.db.QueryRowContext(ctx,
			`SELECT value FROM metrics WHERE id = $1 AND type = 'gauge'`,
			name,
		).Scan(&value)
	}, retry.IsRetriablePG)
	if err != nil {
		// sql.ErrNoRows - не retriable (IsRetriablePG вернёт false и выйдет сразу),
		// но это штатный случай "метрика не найдена", логировать нечего.
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Log.Error("get gauge", zap.String("name", name), zap.Error(err))
		}
		return 0, false
	}
	if !value.Valid {
		return 0, false
	}
	return value.Float64, true
}

func (s *DBStorage) GetCounter(ctx context.Context, name string) (int64, bool) {
	var delta sql.NullInt64
	err := retry.Do(ctx, func() error {
		return s.db.QueryRowContext(ctx,
			`SELECT delta FROM metrics WHERE id = $1 AND type = 'counter'`,
			name,
		).Scan(&delta)
	}, retry.IsRetriablePG)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			logger.Log.Error("get counter", zap.String("name", name), zap.Error(err))
		}
		return 0, false
	}
	if !delta.Valid {
		return 0, false
	}
	return delta.Int64, true
}

// UpdateBatch пишет весь батч одной транзакцией: либо применяются все метрики,
// либо (при ошибке любого Exec) ни одна - частично применённый батч был бы хуже,
// чем явный отказ.
//
// retry.Do оборачивает ВСЮ транзакцию (BeginTx → Exec'и → Commit) единым fn,
// а не отдельные Exec внутри неё: если соединение оборвалось на середине, старая
// транзакция уже не восстановить, повторять нужно с чистого BeginTx.
func (s *DBStorage) UpdateBatch(ctx context.Context, metrics []models.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}

	// Дедуп ПЕРЕД записью: агент может прислать несколько значений одной и той же
	// метрики в одном батче (например, две gauge с одинаковым id). Без дедупа
	// в транзакцию ушло бы несколько ON CONFLICT-апдейтов по одному id подряд -
	// не ошибка сама по себе (это не один multi-row INSERT, а последовательные
	// Exec), но лишние round-trip'ы и путаница, какое значение "победило".
	aggregated := aggregateMetrics(metrics)

	return retry.Do(ctx, func() error {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		// Откат по умолчанию - идиома "defer Rollback, а успешный Commit его обезвредит"
		// (Rollback после Commit вернёт sql.ErrTxDone, но это не проверяется - ошибка ожидаема).
		defer tx.Rollback()

		for _, m := range aggregated {
			switch m.MType {
			case models.Gauge:
				_, err = tx.ExecContext(ctx,
					`INSERT INTO metrics (id, type, value) VALUES ($1, 'gauge', $2)
					 ON CONFLICT (id) DO UPDATE SET value = $2, type = 'gauge'`,
					m.ID, *m.Value,
				)
			case models.Counter:
				_, err = tx.ExecContext(ctx,
					`INSERT INTO metrics (id, type, delta) VALUES ($1, 'counter', $2)
					 ON CONFLICT (id) DO UPDATE SET delta = metrics.delta + $2, type = 'counter'`,
					m.ID, *m.Delta,
				)
			}
			if err != nil {
				return err
			}
		}

		return tx.Commit()
	}, retry.IsRetriablePG)
}

// aggregateMetrics схлопывает дубли по (id, type): для counter суммирует все delta
// из батча в одно значение, для gauge оставляет последнее по порядку value.
// Порядок неучаствовавших в дублировании метрик сохраняется.
func aggregateMetrics(metrics []models.Metrics) []models.Metrics {
	type key struct {
		id    string
		mtype string
	}

	index := make(map[key]int, len(metrics))
	result := make([]models.Metrics, 0, len(metrics))

	for _, m := range metrics {
		k := key{id: m.ID, mtype: m.MType}
		if i, ok := index[k]; ok {
			switch m.MType {
			case models.Counter:
				if m.Delta != nil {
					sum := *result[i].Delta + *m.Delta
					result[i].Delta = &sum
				}
			case models.Gauge:
				result[i] = m
			}
			continue
		}

		index[k] = len(result)
		result = append(result, m)
	}

	return result
}

func (s *DBStorage) Metrics(ctx context.Context) []models.Metrics {
	var result []models.Metrics

	err := retry.Do(ctx, func() error {
		rows, err := s.db.QueryContext(ctx,
			`SELECT id, type, value, delta FROM metrics`,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		// Копим в локальную переменную и присваиваем result только при полном
		// успехе - иначе повтор после частичного прохода цикла задвоил бы записи.
		var batch []models.Metrics
		for rows.Next() {
			var (
				id, mType string
				value     sql.NullFloat64
				delta     sql.NullInt64
			)
			if err := rows.Scan(&id, &mType, &value, &delta); err != nil {
				return err
			}

			m := models.Metrics{ID: id, MType: mType}
			if value.Valid {
				v := value.Float64
				m.Value = &v
			}
			if delta.Valid {
				d := delta.Int64
				m.Delta = &d
			}
			batch = append(batch, m)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		result = batch
		return nil
	}, retry.IsRetriablePG)

	if err != nil {
		logger.Log.Error("list metrics", zap.Error(err))
		return nil
	}

	return result
}
