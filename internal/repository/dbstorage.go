package repository

import (
	"context"
	"database/sql"
	"errors"

	"go.uber.org/zap"

	"github.com/dismoralzor/metalert/internal/logger"
	"github.com/dismoralzor/metalert/internal/model"
)

// DBStorage реализует Storage поверх PostgreSQL: одна таблица metrics,
// значение gauge/counter лежит в колонке value/delta в зависимости от type.
type DBStorage struct {
	db *sql.DB
}

func NewDBStorage(db *sql.DB) *DBStorage {
	return &DBStorage{db: db}
}

// TODO: интерфейс Storage сейчас без context.Context, поэтому здесь
// context.Background() - обсудить пробрасывание ctx из handler'ов через интерфейс.

// Ошибку логируем: интерфейс Storage не позволяет вернуть её вызывающей стороне.
func (s *DBStorage) UpdateGauge(name string, value float64) {
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO metrics (id, type, value) VALUES ($1, 'gauge', $2)
		 ON CONFLICT (id) DO UPDATE SET value = $2, type = 'gauge'`,
		name, value,
	)
	if err != nil {
		logger.Log.Error("update gauge", zap.String("name", name), zap.Error(err))
	}
}

// UpdateCounter прибавляет delta, а не заменяет значение - как и MemStorage.
func (s *DBStorage) UpdateCounter(name string, delta int64) {
	_, err := s.db.ExecContext(context.Background(),
		`INSERT INTO metrics (id, type, delta) VALUES ($1, 'counter', $2)
		 ON CONFLICT (id) DO UPDATE SET delta = metrics.delta + $2, type = 'counter'`,
		name, delta,
	)
	if err != nil {
		logger.Log.Error("update counter", zap.String("name", name), zap.Error(err))
	}
}

func (s *DBStorage) GetGauge(name string) (float64, bool) {
	var value sql.NullFloat64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT value FROM metrics WHERE id = $1 AND type = 'gauge'`,
		name,
	).Scan(&value)
	if err != nil {
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

func (s *DBStorage) GetCounter(name string) (int64, bool) {
	var delta sql.NullInt64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT delta FROM metrics WHERE id = $1 AND type = 'counter'`,
		name,
	).Scan(&delta)
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

func (s *DBStorage) Metrics() []models.Metrics {
	rows, err := s.db.QueryContext(context.Background(),
		`SELECT id, type, value, delta FROM metrics`,
	)
	if err != nil {
		logger.Log.Error("list metrics", zap.Error(err))
		return nil
	}
	defer rows.Close()

	var result []models.Metrics
	for rows.Next() {
		var (
			id, mType string
			value     sql.NullFloat64
			delta     sql.NullInt64
		)
		if err := rows.Scan(&id, &mType, &value, &delta); err != nil {
			logger.Log.Error("scan metric", zap.Error(err))
			return nil
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
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		logger.Log.Error("iterate metrics", zap.Error(err))
		return nil
	}

	return result
}
