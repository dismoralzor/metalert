package repository

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/dismoralzor/metalert/internal/model"
)

func ptrFloat(v float64) *float64 { return &v }
func ptrInt(v int64) *int64       { return &v }

func TestDBStorage_UpdateBatch_CommitsTransaction(t *testing.T) {
	s, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 23.5).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO metrics \(id, type, delta\) VALUES \(\$1, 'counter', \$2\)`).
		WithArgs("requests", int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	metrics := []models.Metrics{
		{ID: "temperature", MType: models.Gauge, Value: ptrFloat(23.5)},
		{ID: "requests", MType: models.Counter, Delta: ptrInt(5)},
	}

	if err := s.UpdateBatch(context.Background(), metrics); err != nil {
		t.Fatalf("UpdateBatch() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBStorage_UpdateBatch_RollsBackOnExecError(t *testing.T) {
	s, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 23.5).
		WillReturnError(errors.New("boom"))
	mock.ExpectRollback()

	metrics := []models.Metrics{
		{ID: "temperature", MType: models.Gauge, Value: ptrFloat(23.5)},
	}

	if err := s.UpdateBatch(context.Background(), metrics); err == nil {
		t.Fatal("UpdateBatch() error = nil, want error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Дубли counter с одинаковым id должны схлопнуться в ОДИН Exec с суммой delta -
// без предварительной агрегации sqlmock увидел бы два отдельных запроса.
func TestDBStorage_UpdateBatch_AggregatesDuplicateCounters(t *testing.T) {
	s, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO metrics \(id, type, delta\) VALUES \(\$1, 'counter', \$2\)`).
		WithArgs("requests", int64(8)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	metrics := []models.Metrics{
		{ID: "requests", MType: models.Counter, Delta: ptrInt(5)},
		{ID: "requests", MType: models.Counter, Delta: ptrInt(3)},
	}

	if err := s.UpdateBatch(context.Background(), metrics); err != nil {
		t.Fatalf("UpdateBatch() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// Дубли gauge с одинаковым id - побеждает последнее по порядку значение.
func TestDBStorage_UpdateBatch_AggregatesDuplicateGaugesKeepsLast(t *testing.T) {
	s, mock := newMockStorage(t)

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 30.0).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	metrics := []models.Metrics{
		{ID: "temperature", MType: models.Gauge, Value: ptrFloat(23.5)},
		{ID: "temperature", MType: models.Gauge, Value: ptrFloat(30.0)},
	}

	if err := s.UpdateBatch(context.Background(), metrics); err != nil {
		t.Fatalf("UpdateBatch() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBStorage_UpdateBatch_EmptyBatchIsNoop(t *testing.T) {
	s, _ := newMockStorage(t)

	if err := s.UpdateBatch(context.Background(), nil); err != nil {
		t.Fatalf("UpdateBatch() error = %v", err)
	}
}
