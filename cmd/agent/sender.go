package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/dismoralzor/metalert/internal/hash"
	models "github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/retry"
)

// sender сворачивает всё, что нужно для отправки батча: HTTP-клиент, адрес
// сервера, ключ подписи и политику повторов. Один экземпляр делят воркеры и
// финальная отправка при shutdown.
type sender struct {
	client *http.Client
	addr   string
	key    string
	policy retry.Policy
}

// newSender создаёт отправитель с заданной политикой повторов.
func newSender(client *http.Client, addr, key string, policy retry.Policy) *sender {
	return &sender{client: client, addr: addr, key: key, policy: policy}
}

// Send отправляет батч с обычной политикой повторов. Возвращает true при успехе;
// ошибку логирует сам (воркеру достаточно bool для sendResult).
func (s *sender) Send(ctx context.Context, metrics []models.Metrics) bool {
	if err := s.send(ctx, s.policy, metrics); err != nil {
		log.Printf("send batch: %v", err)
		return false
	}
	return true
}

// SendOnce делает ровно одну попытку, без повторов (пустая политика). Нужна для
// финальной отправки при shutdown: основной ctx уже отменён, ждать между
// попытками бессмысленно. Ошибку возвращает вызывающей стороне для логирования.
func (s *sender) SendOnce(ctx context.Context, metrics []models.Metrics) error {
	return s.send(ctx, retry.Policy{}, metrics)
}

// send отправляет весь батч одним JSON-массивом, сжатым gzip, на POST /updates/.
//
// Порядок важен: подпись считается от СЫРОГО JSON, ДО gzip - сервер распаковывает
// тело раньше, чем проверяет HashSHA256, значит и агент обязан подписывать то же,
// несжатое, тело. При пустом key заголовок не добавляется.
//
// Маршалинг, подпись и gzip делаем один раз ДО policy.Do - они детерминированы,
// повторять их незачем; тело запроса на каждую попытку пересобираем из уже сжатых
// байт через bytes.Reader (bytes.Buffer http-клиент "съедает" при первом чтении).
func (s *sender) send(ctx context.Context, policy retry.Policy, metrics []models.Metrics) error {
	body, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	var signature string
	if s.key != "" {
		signature = hash.Compute(body, s.key)
	}

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(body); err != nil {
		return fmt.Errorf("compress batch: %w", err)
	}
	// Close до отправки: он дописывает хвост gzip-потока, без него сервер получит
	// обрезанные данные.
	if err := zw.Close(); err != nil {
		return fmt.Errorf("compress batch: %w", err)
	}
	compressedBody := compressed.Bytes()

	url := fmt.Sprintf("http://%s/updates/", s.addr)

	return policy.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(compressedBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		if signature != "" {
			req.Header.Set("HashSHA256", signature)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		// Дочитываем тело ДО Close - иначе транспорт не переиспользует TCP-соединение.
		io.Copy(io.Discard, resp.Body)

		// client.Do возвращает err только при сбое транспорта - код ответа сервера
		// (500/400) сюда не попадает и проверяется отдельно, иначе любой HTTP-ответ
		// считался бы удачной отправкой.
		if resp.StatusCode < http.StatusOK || resp.StatusCode > 299 {
			return &retry.HTTPStatusError{StatusCode: resp.StatusCode}
		}
		return nil
	}, retry.IsRetriableNet)
}
