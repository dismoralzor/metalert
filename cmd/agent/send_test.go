package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/retry"
)

// withShortRetryDelays подменяет retry.Delays на короткие интервалы на время
// теста, чтобы не ждать реальные 1s+3s+5s при проверке retriable-путей.
func withShortRetryDelays(t *testing.T) {
	t.Helper()
	original := retry.Delays
	retry.Delays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retry.Delays = original })
}

func TestSendBatch_2xxIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ok := sendBatch(context.Background(), srv.Client(), srv.Listener.Addr().String(), "", []models.Metrics{})
	if !ok {
		t.Error("sendBatch() = false, want true for 200 response")
	}
}

// Раньше client.Do возвращал err == nil для любого HTTP-ответа, включая 500,
// и sendBatch молча считал это успехом - батч (и pollDelta) терялся.
func TestSendBatch_5xxIsFailureAndRetries(t *testing.T) {
	withShortRetryDelays(t)

	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ok := sendBatch(context.Background(), srv.Client(), srv.Listener.Addr().String(), "", []models.Metrics{})
	if ok {
		t.Error("sendBatch() = true, want false for 500 response")
	}

	want := int32(1 + len(retry.Delays))
	if got := atomic.LoadInt32(&attempts); got != want {
		t.Errorf("attempts = %d, want %d (основной запрос + %d повтора на 5xx)", got, want, len(retry.Delays))
	}
}

// 4xx - ошибка самого запроса, повтор её не исправит: sendBatch должен
// вернуть false с ОДНОЙ попытки, не тратя время на retry.
func TestSendBatch_4xxIsFailureWithoutRetry(t *testing.T) {
	withShortRetryDelays(t)

	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	ok := sendBatch(context.Background(), srv.Client(), srv.Listener.Addr().String(), "", []models.Metrics{})
	if ok {
		t.Error("sendBatch() = true, want false for 400 response")
	}

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts = %d, want 1 (без повтора на 4xx)", got)
	}
}

// Интеграция с worker/accumulator: неудачная отправка (5xx) не должна терять
// pollDelta - runWorker сообщает об этом через resultsCh, а не проглатывает.
func TestSendBatch_FailureSurfacesToWorkerResult(t *testing.T) {
	withShortRetryDelays(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobsCh := make(chan job, 1)
	resultsCh := make(chan sendResult, 1)

	client := srv.Client()
	go runWorker(ctx, client, srv.Listener.Addr().String(), "", jobsCh, resultsCh, sendBatch)

	jobsCh <- job{metrics: []models.Metrics{}, pollDelta: 7}
	close(jobsCh)

	select {
	case res := <-resultsCh:
		if res.success {
			t.Error("sendResult.success = true, want false for 500 response")
		}
		if res.pollDelta != 7 {
			t.Errorf("sendResult.pollDelta = %d, want 7 (не потерян)", res.pollDelta)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for sendResult")
	}
}
