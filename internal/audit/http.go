package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HTTPObserver шлёт события аудита POST-запросом на внешний URL.
type HTTPObserver struct {
	url    string
	client *http.Client
}

// NewHTTPObserver создаёт приёмник, отправляющий события на url.
// Клиент с таймаутом: NotifyAll синхронен, зависший приёмник не должен
// задерживать ответ сервера клиенту.
func NewHTTPObserver(url string) *HTTPObserver {
	return &HTTPObserver{
		url:    url,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

// Notify сериализует событие и шлёт его POST'ом с JSON-телом. Тело ответа
// дочитываем и закрываем, чтобы соединение вернулось в пул keep-alive.
func (o *HTTPObserver) Notify(e Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, o.url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("audit receiver returned %s", resp.Status)
	}
	return nil
}

// Close освобождает простаивающие keep-alive соединения клиента. Вызывается
// Subject.Close при остановке сервера.
func (o *HTTPObserver) Close() error {
	o.client.CloseIdleConnections()
	return nil
}
