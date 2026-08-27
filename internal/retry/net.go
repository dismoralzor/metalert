package retry

import (
	"errors"
	"io"
	"net"
	"syscall"
)

// IsRetriableNet считает retriable сетевые ошибки при отправке метрик агентом:
// сервер временно недоступен (connection refused, таймаут, обрыв соединения) -
// в отличие, например, от ошибки маршалинга тела запроса, повтор того же запроса
// имеет смысл.
func IsRetriableNet(err error) bool {
	if err == nil {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, io.EOF) {
		return true
	}

	return false
}
