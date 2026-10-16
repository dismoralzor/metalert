package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	models "github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/retry"
)

// shortPolicy - политика с миллисекундными паузами: тесты на retry не должны
// ждать реальные 1s+3s+5s. Своя на каждый тест, без общего состояния.
func shortPolicy() retry.Policy {
	return retry.Policy{Delays: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}}
}

func TestSender_2xxIsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newSender(srv.Client(), srv.Listener.Addr().String(), "", shortPolicy())
	if !s.Send(context.Background(), []models.Metrics{}) {
		t.Error("Send() = false, want true for 200 response")
	}
}

// Раньше client.Do возвращал err == nil для любого HTTP-ответа, включая 500,
// и отправка молча считалась успехом - батч (и pollDelta) терялся.
func TestSender_5xxIsFailureAndRetries(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	policy := shortPolicy()
	s := newSender(srv.Client(), srv.Listener.Addr().String(), "", policy)
	if s.Send(context.Background(), []models.Metrics{}) {
		t.Error("Send() = true, want false for 500 response")
	}

	want := int32(1 + len(policy.Delays))
	if got := atomic.LoadInt32(&attempts); got != want {
		t.Errorf("attempts = %d, want %d (основной запрос + %d повтора на 5xx)", got, want, len(policy.Delays))
	}
}

// 4xx - ошибка самого запроса, повтор её не исправит: Send должен вернуть false
// с ОДНОЙ попытки, не тратя время на retry.
func TestSender_4xxIsFailureWithoutRetry(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	s := newSender(srv.Client(), srv.Listener.Addr().String(), "", shortPolicy())
	if s.Send(context.Background(), []models.Metrics{}) {
		t.Error("Send() = true, want false for 400 response")
	}

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts = %d, want 1 (без повтора на 4xx)", got)
	}
}

// SendOnce делает ровно одну попытку даже на retriable-ошибку (5xx) - это путь
// финальной отправки при shutdown.
func TestSender_SendOnceDoesNotRetry(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := newSender(srv.Client(), srv.Listener.Addr().String(), "", shortPolicy())
	if err := s.SendOnce(context.Background(), []models.Metrics{}); err == nil {
		t.Error("SendOnce() = nil, want error for 500 response")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("attempts = %d, want 1 (SendOnce без повторов)", got)
	}
}

// Интеграция с worker: неудачная отправка (5xx) не должна терять pollDelta -
// runWorker сообщает об этом через resultsCh, а не проглатывает.
func TestSender_FailureSurfacesToWorkerResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	jobsCh := make(chan job, 1)
	resultsCh := make(chan sendResult, 1)

	s := newSender(srv.Client(), srv.Listener.Addr().String(), "", shortPolicy())
	go runWorker(ctx, jobsCh, resultsCh, s.Send)

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
