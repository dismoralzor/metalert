package repository

import (
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func newMockStorage(t *testing.T) (*DBStorage, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewDBStorage(db), mock
}

func TestDBStorage_UpdateGauge(t *testing.T) {
	s, mock := newMockStorage(t)

	mock.ExpectExec(`INSERT INTO metrics \(id, type, value\) VALUES \(\$1, 'gauge', \$2\)`).
		WithArgs("temperature", 23.5).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.UpdateGauge("temperature", 23.5)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBStorage_UpdateCounter_Accumulates(t *testing.T) {
	s, mock := newMockStorage(t)

	mock.ExpectExec(`INSERT INTO metrics \(id, type, delta\) VALUES \(\$1, 'counter', \$2\)`).
		WithArgs("requests", int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO metrics \(id, type, delta\) VALUES \(\$1, 'counter', \$2\)`).
		WithArgs("requests", int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s.UpdateCounter("requests", 10)
	s.UpdateCounter("requests", 5)

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestDBStorage_GetGauge(t *testing.T) {
	t.Run("existing", func(t *testing.T) {
		s, mock := newMockStorage(t)

		rows := sqlmock.NewRows([]string{"value"}).AddRow(23.5)
		mock.ExpectQuery(`SELECT value FROM metrics WHERE id = \$1 AND type = 'gauge'`).
			WithArgs("temperature").
			WillReturnRows(rows)

		value, ok := s.GetGauge("temperature")
		if !ok || value != 23.5 {
			t.Errorf("GetGauge() = %v, %v, want 23.5, true", value, ok)
		}
	})

	t.Run("missing", func(t *testing.T) {
		s, mock := newMockStorage(t)

		mock.ExpectQuery(`SELECT value FROM metrics WHERE id = \$1 AND type = 'gauge'`).
			WithArgs("unknown").
			WillReturnRows(sqlmock.NewRows([]string{"value"}))

		_, ok := s.GetGauge("unknown")
		if ok {
			t.Errorf("GetGauge() ok = true, want false")
		}
	})
}

func TestDBStorage_GetCounter(t *testing.T) {
	t.Run("existing", func(t *testing.T) {
		s, mock := newMockStorage(t)

		rows := sqlmock.NewRows([]string{"delta"}).AddRow(int64(15))
		mock.ExpectQuery(`SELECT delta FROM metrics WHERE id = \$1 AND type = 'counter'`).
			WithArgs("requests").
			WillReturnRows(rows)

		delta, ok := s.GetCounter("requests")
		if !ok || delta != 15 {
			t.Errorf("GetCounter() = %v, %v, want 15, true", delta, ok)
		}
	})

	t.Run("missing", func(t *testing.T) {
		s, mock := newMockStorage(t)

		mock.ExpectQuery(`SELECT delta FROM metrics WHERE id = \$1 AND type = 'counter'`).
			WithArgs("unknown").
			WillReturnRows(sqlmock.NewRows([]string{"delta"}))

		_, ok := s.GetCounter("unknown")
		if ok {
			t.Errorf("GetCounter() ok = true, want false")
		}
	})
}

func TestDBStorage_Metrics(t *testing.T) {
	s, mock := newMockStorage(t)

	rows := sqlmock.NewRows([]string{"id", "type", "value", "delta"}).
		AddRow("temperature", "gauge", 23.5, nil).
		AddRow("requests", "counter", nil, int64(10))
	mock.ExpectQuery(`SELECT id, type, value, delta FROM metrics`).
		WillReturnRows(rows)

	result := s.Metrics()

	if len(result) != 2 {
		t.Fatalf("Metrics() len = %d, want 2", len(result))
	}

	gauge, counter := result[0], result[1]
	if gauge.ID != "temperature" || gauge.MType != "gauge" || gauge.Value == nil || *gauge.Value != 23.5 {
		t.Errorf("Metrics()[0] = %+v", gauge)
	}
	if counter.ID != "requests" || counter.MType != "counter" || counter.Delta == nil || *counter.Delta != 10 {
		t.Errorf("Metrics()[1] = %+v", counter)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
