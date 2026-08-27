package retry

import (
	"errors"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// IsRetriablePG считает retriable только ошибки Class 08 - Connection Exception:
// соединение с БД временно недоступно/разорвано. Всё остальное (нарушение
// constraint, синтаксическая ошибка в SQL и т.п.) - детерминированная ошибка,
// повтор того же запроса даст тот же результат.
func IsRetriablePG(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	switch pgErr.Code {
	case pgerrcode.ConnectionException,
		pgerrcode.ConnectionDoesNotExist,
		pgerrcode.ConnectionFailure,
		pgerrcode.SQLClientUnableToEstablishSQLConnection,
		pgerrcode.SQLServerRejectedEstablishmentOfSQLConnection:
		return true
	default:
		return false
	}
}
