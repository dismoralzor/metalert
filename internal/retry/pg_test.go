package retry

import (
	"errors"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsRetriablePG(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"connection exception", &pgconn.PgError{Code: pgerrcode.ConnectionException}, true},
		{"connection does not exist", &pgconn.PgError{Code: pgerrcode.ConnectionDoesNotExist}, true},
		{"connection failure", &pgconn.PgError{Code: pgerrcode.ConnectionFailure}, true},
		{"client unable to establish", &pgconn.PgError{Code: pgerrcode.SQLClientUnableToEstablishSQLConnection}, true},
		{"server rejected establishment", &pgconn.PgError{Code: pgerrcode.SQLServerRejectedEstablishmentOfSQLConnection}, true},
		{"unique violation - not retriable", &pgconn.PgError{Code: pgerrcode.UniqueViolation}, false},
		{"syntax error - not retriable", &pgconn.PgError{Code: pgerrcode.SyntaxError}, false},
		{"non-pg error", errors.New("boom"), false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetriablePG(tt.err); got != tt.want {
				t.Errorf("IsRetriablePG(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// Ошибка, обёрнутая через fmt.Errorf("%w", ...) или чужой враппер, должна
// распознаваться через errors.As, а не требовать точного типа на верхнем уровне.
func TestIsRetriablePG_WrappedError(t *testing.T) {
	wrapped := wrapErr{err: &pgconn.PgError{Code: pgerrcode.ConnectionFailure}}
	if !IsRetriablePG(wrapped) {
		t.Error("IsRetriablePG() = false, want true for wrapped retriable error")
	}
}

type wrapErr struct{ err error }

func (w wrapErr) Error() string { return w.err.Error() }
func (w wrapErr) Unwrap() error { return w.err }
