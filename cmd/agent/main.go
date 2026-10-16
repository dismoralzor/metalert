// Command agent - агент сбора метрик: периодически опрашивает runtime- и
// системные метрики и батчами отправляет их на сервер по HTTP.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	models "github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/retry"
)

// agent хранит только pollCount: горутина A инкрементирует его на каждый опрос,
// аккумулятор оптимистично забирает его в батч и возвращает обратно при неудаче
// отправки (см. runAccumulator). Сами метрики больше не копятся в структуре -
// они летят по metricsCh сразу после сбора.
//
// atomic.Int64, а не мьютекс: единственная операция, которая тут нужна помимо
// простого чтения/записи - "забрать текущее значение и обнулить", это ровно
// Swap(0), без надобности в отдельной блокировке.
type agent struct {
	pollCount atomic.Int64
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
	// sender делят воркеры (обычная отправка с retry) и финальный flush
	// аккумулятора при shutdown (одна попытка без retry).
	s := newSender(client, cfg.addr, cfg.key, retry.DefaultPolicy())

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
		runAccumulator(ctx, cfg.reportInterval, metricsCh, jobsCh, resultsCh, a, s)
	}()

	// Пул воркеров: не более cfg.rateLimit одновременных исходящих запросов.
	for i := 0; i < cfg.rateLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(ctx, jobsCh, resultsCh, s.Send)
		}()
	}

	// Ждём сигнала, а затем - пока все горутины реально завершатся
	// (а не просто "получили сигнал и продолжают тикать где-то в фоне").
	<-ctx.Done()
	wg.Wait()
}
