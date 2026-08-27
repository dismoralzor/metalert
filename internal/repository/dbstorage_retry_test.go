package repository

import (
	"context"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/retry"
)

// withShortRetryDelays подменяет retry.Delays на короткие интервалы на время
// теста, чтобы не ждать реальные 1s+3s+5s.
func withShortRetryDelays(t *testing.T) {
	t.Helper()
	original := retry.Delays
	retry.Delays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retry.Delays = original })
}

func connExceptionErr() error {
	return &pgconn.PgError{Code: pgerrcode.ConnectionFailure}
}

// Class 08 (Connection Exception) - retriable: второй Exec должен пройти без ошибки.
func TestDBStorage_UpdateGauge_RetriesOnConnectionException(t *testing.T) {
	withShortRetryDelays(t)
	s, mock := newMockStorage(t)

	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 23.5).
		WillReturnError(connExceptionErr())
	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 23.5).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.UpdateGauge("temperature", 23.5)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (expected exactly 2 Exec calls): %v", err)
	}
}

// Ошибка вне Class 08 (например, нарушение constraint) - НЕ retriable,
// повторного Exec быть не должно.
func TestDBStorage_UpdateGauge_DoesNotRetryOnNonConnectionError(t *testing.T) {
	withShortRetryDelays(t)
	s, mock := newMockStorage(t)

	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 23.5).
		WillReturnError(&pgconn.PgError{Code: pgerrcode.UniqueViolation})

	s.UpdateGauge("temperature", 23.5)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (expected exactly 1 Exec call, no retry): %v", err)
	}
}

// UpdateBatch: обрыв соединения происходит ПОСЛЕ BeginTx, но retry обязан
// перезапустить ВСЮ транзакцию (новый BeginTx), а не пытаться доExec'ать
// в уже мёртвую tx.
func TestDBStorage_UpdateBatch_RetriesWholeTransaction(t *testing.T) {
	withShortRetryDelays(t)
	s, mock := newMockStorage(t)

	// Первая попытка: транзакция открылась, но упала на первом Exec.
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 23.5).
		WillReturnError(connExceptionErr())
	mock.ExpectRollback()

	// Вторая попытка: новый BeginTx, весь батч заново, и уже успешно.
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 23.5).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	metrics := []models.Metrics{
		{ID: "temperature", MType: models.Gauge, Value: ptrFloat(23.5)},
	}

	if err := s.UpdateBatch(context.Background(), metrics); err != nil {
		t.Fatalf("UpdateBatch() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (expected BeginTx retried from scratch): %v", err)
	}
}
