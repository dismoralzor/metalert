package handler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestPingHandler_Ping(t *testing.T) {
	t.Run("db not configured", func(t *testing.T) {
		h := NewPingHandler(nil)

		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		h.Ping(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Ping() code = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})

	t.Run("db reachable", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer db.Close()
		mock.ExpectPing()

		h := NewPingHandler(db)

		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		h.Ping(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Ping() code = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("db unreachable", func(t *testing.T) {
		db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
		if err != nil {
			t.Fatalf("sqlmock.New: %v", err)
		}
		defer db.Close()
		mock.ExpectPing().WillReturnError(sql.ErrConnDone)

		h := NewPingHandler(db)

		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w := httptest.NewRecorder()
		h.Ping(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("Ping() code = %d, want %d", w.Code, http.StatusInternalServerError)
		}
	})
}
