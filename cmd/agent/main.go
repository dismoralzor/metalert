package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dismoralzor/metalert/internal/hash"
	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/retry"
)

// agent хранит только pollCount: горутина A инкрементирует его на каждый опрос,
// аккумулятор оптимистично забирает его в батч и возвращает обратно при неудаче
// отправки (см. runAccumulator). Сами метрики больше не копятся в структуре -
// они летят по metricsCh сразу после сбора.
type agent struct {
	mu        sync.Mutex
	pollCount int64
}

// sendBatch отправляет весь батч метрик одним JSON-массивом, сжатым gzip,
// на POST /updates/ - вместо отдельного запроса на каждую метрику.
//
// Порядок важен: подпись считается от СЫРОГО JSON, ДО gzip - сервер распаковывает
// тело раньше, чем проверяет HashSHA256, значит и агент обязан подписывать то же,
// несжатое, тело. При пустом key заголовок вообще не добавляется - поведение как
// до инкремента.
//
// Сетевую часть (сам POST) оборачиваем в retry.Do: если сервер временно
// недоступен (connection refused, обрыв, таймаут), повторяем запрос ещё
// до 3 раз с паузами 1s/3s/5s. Маршалинг, подпись и gzip делаем один раз ДО
// retry - они детерминированы, повторять их не за чем, но тело запроса на
// каждую попытку пересобираем заново из уже сжатых байт: bytes.Reader,
// в отличие от bytes.Buffer, http-клиент "съедает" при первом же чтении.
func sendBatch(ctx context.Context, client *http.Client, addr, key string, metrics []models.Metrics) bool {
	body, err := json.Marshal(metrics)
	if err != nil {
		log.Printf("marshal batch: %v", err)
		return false
	}

	var signature string
	if key != "" {
		signature = hash.Compute(body, key)
	}

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(body); err != nil {
		log.Printf("compress batch: %v", err)
		return false
	}
	// Close до отправки, а не через defer: он дописывает хвост gzip-потока,
	// без него сервер получит обрезанные данные.
	if err := zw.Close(); err != nil {
		log.Printf("compress batch: %v", err)
		return false
	}
	compressedBody := compressed.Bytes()

	url := fmt.Sprintf("http://%s/updates/", addr)

	err = retry.Do(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(compressedBody))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")
		if signature != "" {
			req.Header.Set("HashSHA256", signature)
		}

		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		return nil
	}, retry.IsRetriableNet)

	if err != nil {
		log.Printf("send batch: %v", err)
		return false
	}
	return true
}

func main() {
	cfg := parseFlags()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SIGTERM - обычный сигнал от docker stop / systemd, SIGINT - Ctrl+C.
	// Один cancel() на оба - всем горутинам ниже всё равно, каким сигналом их остановили.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	a := &agent{}
	client := &http.Client{Timeout: 5 * time.Second}

	// metricsCh - выход горутин A и B, вход аккумулятора. jobsCh/resultsCh -
	// обвязка между аккумулятором и пулом воркеров. Буферы небольшие: достаточно
	// сгладить всплеск в момент отправки батча, не более того - основное
	// ограничение конкурентности даёт число воркеров, а не размер буфера.
	metricsCh := make(chan models.Metrics, 64)
	jobsCh := make(chan job, cfg.rateLimit)
	resultsCh := make(chan sendResult, cfg.rateLimit)

	var wg sync.WaitGroup

	// Горутина A: runtime-метрики.
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectRuntimeMetrics(ctx, cfg.pollInterval, metricsCh, a)
	}()

	// Горутина B: gopsutil-метрики, свой тикер, независимо от A.
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectPSUtilMetrics(ctx, cfg.pollInterval, metricsCh)
	}()

	// Горутина-аккумулятор: копит metricsCh, по reportInterval формирует батч.
	wg.Add(1)
	go func() {
		defer wg.Done()
		runAccumulator(ctx, cfg.reportInterval, metricsCh, jobsCh, resultsCh, a)
	}()

	// Пул воркеров: не более cfg.rateLimit одновременных исходящих запросов.
	for i := 0; i < cfg.rateLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(ctx, client, cfg.addr, cfg.key, jobsCh, resultsCh, sendBatch)
		}()
	}

	// Ждём сигнала, а затем - пока все горутины реально завершатся
	// (а не просто "получили сигнал и продолжают тикать где-то в фоне").
	<-ctx.Done()
	wg.Wait()
}
