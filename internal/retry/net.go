package retry

import (
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
)

// HTTPStatusError оборачивает HTTP-ответ с кодом вне диапазона 2xx. client.Do
// возвращает err только при сбое транспорта (не смог соединиться, оборвалось
// и т.п.) - код ответа сервера (например 500) сам по себе НЕ ошибка с точки
// зрения http.Client, поэтому вызывающая сторона обязана проверить StatusCode
// и завернуть неуспех в ошибку сама, иначе retry.Do её просто не увидит.
type HTTPStatusError struct {
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("unexpected status code %d", e.StatusCode)
}

// IsRetriableNet считает retriable:
//   - сетевые ошибки (net.Error, ECONNREFUSED, io.EOF) - сервер временно
//     недоступен, повтор того же запроса имеет смысл;
//   - HTTPStatusError с кодом 5xx - временная ошибка НА СТОРОНЕ сервера.
//
// HTTPStatusError с кодом 4xx НЕ retriable: это ошибка самого запроса
// (например, невалидное тело или подпись), повтор того же запроса даст
// тот же результат.
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

	var statusErr *HTTPStatusError
	if errors.As(err, &statusErr) {
		return statusErr.StatusCode >= 500 && statusErr.StatusCode <= 599
	}

	return false
}
