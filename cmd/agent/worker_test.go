package main

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/dismoralzor/metalert/internal/model"
)

// Пул из rateLimit воркеров, заваленный jobs'ами, не должен допускать больше
// rateLimit одновременных вызовов send - это и есть смысл ограничения
// параллельных исходящих запросов.
func TestWorkerPool_RespectsRateLimit(t *testing.T) {
	const rateLimit = 3
	const totalJobs = 12

	var (
		mu      sync.Mutex
		current int
		maxSeen int
	)

	fakeSend := func(ctx context.Context, client *http.Client, addr, key string, metrics []models.Metrics) bool {
		mu.Lock()
		current++
		if current > maxSeen {
			maxSeen = current
		}
		mu.Unlock()

		time.Sleep(30 * time.Millisecond)

		mu.Lock()
		current--
		mu.Unlock()
		return true
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobsCh := make(chan job, totalJobs)
	resultsCh := make(chan sendResult, totalJobs)

	var wg sync.WaitGroup
	for i := 0; i < rateLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(ctx, nil, "", "", jobsCh, resultsCh, fakeSend)
		}()
	}

	for i := 0; i < totalJobs; i++ {
		jobsCh <- job{pollDelta: int64(i)}
	}
	close(jobsCh)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker pool did not finish in time")
	}

	if maxSeen > rateLimit {
		t.Errorf("max concurrent sends = %d, want <= %d", maxSeen, rateLimit)
	}
	if maxSeen < 1 {
		t.Error("no concurrent sends observed - test setup issue")
	}

	if len(resultsCh) != totalJobs {
		t.Errorf("resultsCh has %d results, want %d", len(resultsCh), totalJobs)
	}
}

// Воркер обязан завершиться по ctx.Done(), даже если jobsCh ещё не закрыт
// и в нём не осталось заданий (иначе процесс агента не смог бы выйти при shutdown).
func TestRunWorker_StopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	jobsCh := make(chan job)
	resultsCh := make(chan sendResult)

	done := make(chan struct{})
	go func() {
		runWorker(ctx, nil, "", "", jobsCh, resultsCh, func(context.Context, *http.Client, string, string, []models.Metrics) bool {
			return true
		})
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}
