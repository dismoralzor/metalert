package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	models "github.com/dismoralzor/metalert/internal/model"
)

func TestRunAccumulator_FormsJobOnTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricsCh := make(chan models.Metrics)
	jobsCh := make(chan job, 1)
	resultsCh := make(chan sendResult)
	a := &agent{}

	go runAccumulator(ctx, 20*time.Millisecond, metricsCh, jobsCh, resultsCh, a, nil)

	v := 1.0
	metricsCh <- models.Metrics{ID: "Alloc", MType: models.Gauge, Value: &v}

	select {
	case j := <-jobsCh:
		// Alloc + обязательный PollCount, даже если он с delta=0.
		if len(j.metrics) != 2 {
			t.Fatalf("job.metrics len = %d, want 2 (Alloc + PollCount)", len(j.metrics))
		}
		foundAlloc, foundPollCount := false, false
		for _, m := range j.metrics {
			switch m.ID {
			case "Alloc":
				foundAlloc = true
			case "PollCount":
				foundPollCount = true
				if m.MType != models.Counter || m.Delta == nil {
					t.Errorf("PollCount metric malformed: %+v", m)
				}
			}
		}
		if !foundAlloc || !foundPollCount {
			t.Errorf("job.metrics = %+v, missing Alloc or PollCount", j.metrics)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job on jobsCh")
	}
}

// "если слайс пуст - continue": без поступивших метрик тики не должны
// формировать пустые батчи.
func TestRunAccumulator_SkipsEmptyTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricsCh := make(chan models.Metrics)
	jobsCh := make(chan job, 1)
	resultsCh := make(chan sendResult)
	a := &agent{}

	go runAccumulator(ctx, 15*time.Millisecond, metricsCh, jobsCh, resultsCh, a, nil)

	select {
	case j := <-jobsCh:
		t.Fatalf("unexpected job on empty pending slice: %+v", j)
	case <-time.After(150 * time.Millisecond):
		// Несколько тиков прошло, батчей быть не должно - ожидаемо.
	}
}

// pollCount при неудачной отправке должен вернуться (плюс то, что накопилось
// сверху), а не потеряться - иначе неотправленные опросы молча исчезнут.
func TestRunAccumulator_RestoresPollCountOnFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricsCh := make(chan models.Metrics)
	jobsCh := make(chan job, 1)
	resultsCh := make(chan sendResult, 1)
	a := &agent{}
	a.pollCount.Store(5)

	// Интервал намеренно большой - тикер не должен успеть сработать за время теста,
	// проверяем именно обработку sendResult, а не формирование батча.
	go runAccumulator(ctx, time.Hour, metricsCh, jobsCh, resultsCh, a, nil)

	resultsCh <- sendResult{pollDelta: 3, success: false}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := a.pollCount.Load(); got == 8 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Errorf("pollCount = %d, want 8 (5 + restored 3)", a.pollCount.Load())
}

// Успешная отправка НЕ должна ничего добавлять обратно - delta уже была
// вычтена оптимистично в момент формирования батча.
func TestRunAccumulator_DoesNotRestorePollCountOnSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricsCh := make(chan models.Metrics)
	jobsCh := make(chan job, 1)
	resultsCh := make(chan sendResult, 1)
	a := &agent{}
	a.pollCount.Store(5)

	go runAccumulator(ctx, time.Hour, metricsCh, jobsCh, resultsCh, a, nil)

	resultsCh <- sendResult{pollDelta: 3, success: true}

	time.Sleep(100 * time.Millisecond)

	if got := a.pollCount.Load(); got != 5 {
		t.Errorf("pollCount = %d, want 5 (unchanged on success)", got)
	}
}

// При отмене контекста накопленное не теряется: аккумулятор делает финальную
// отправку (одна попытка, отдельный контекст) с pending + PollCount.
func TestRunAccumulator_FlushesPendingOnShutdown(t *testing.T) {
	received := make(chan []models.Metrics, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		raw, _ := io.ReadAll(zr)
		var batch []models.Metrics
		_ = json.Unmarshal(raw, &batch)
		received <- batch
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())

	metricsCh := make(chan models.Metrics)
	jobsCh := make(chan job, 1)
	resultsCh := make(chan sendResult)
	a := &agent{}
	a.pollCount.Store(4)

	s := newSender(srv.Client(), srv.Listener.Addr().String(), "", shortPolicy())

	done := make(chan struct{})
	go func() {
		// reportInterval большой: обычный тик не сработает, проверяем именно
		// финальную отправку при ctx.Done().
		runAccumulator(ctx, time.Hour, metricsCh, jobsCh, resultsCh, a, s)
		close(done)
	}()

	v := 1.0
	metricsCh <- models.Metrics{ID: "Alloc", MType: models.Gauge, Value: &v}
	cancel()

	select {
	case batch := <-received:
		var foundAlloc, foundPoll bool
		for _, m := range batch {
			switch m.ID {
			case "Alloc":
				foundAlloc = true
			case "PollCount":
				foundPoll = true
				if m.Delta == nil || *m.Delta != 4 {
					t.Errorf("PollCount delta = %v, want 4", m.Delta)
				}
			}
		}
		if !foundAlloc || !foundPoll {
			t.Errorf("final batch = %+v, want Alloc + PollCount", batch)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("final batch was not sent on shutdown")
	}

	<-done
}
