package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

// shortPolicy - политика с миллисекундными паузами: тест на "исчерпание попыток"
// не должен ждать реальные 1s+3s+5s. Каждый тест создаёт свою - общего
// изменяемого состояния нет, поэтому тесты безопасно идут через t.Parallel.
func shortPolicy() Policy {
	return Policy{Delays: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}}
}

func TestDo_SucceedsFirstTry(t *testing.T) {
	t.Parallel()

	calls := 0
	err := shortPolicy().Do(context.Background(), func() error {
		calls++
		return nil
	}, func(error) bool { return true })

	if err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

func TestDo_SucceedsAfterRetry(t *testing.T) {
	t.Parallel()

	retriableErr := errors.New("temporary")
	calls := 0
	err := shortPolicy().Do(context.Background(), func() error {
		calls++
		if calls < 3 {
			return retriableErr
		}
		return nil
	}, func(error) bool { return true })

	if err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestDo_NonRetriableReturnsImmediately(t *testing.T) {
	t.Parallel()

	nonRetriableErr := errors.New("permanent")
	calls := 0
	err := shortPolicy().Do(context.Background(), func() error {
		calls++
		return nonRetriableErr
	}, func(error) bool { return false })

	if !errors.Is(err, nonRetriableErr) {
		t.Fatalf("Do() error = %v, want %v", err, nonRetriableErr)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retries for non-retriable error)", calls)
	}
}

func TestDo_ExhaustsRetries(t *testing.T) {
	t.Parallel()

	policy := shortPolicy()
	retriableErr := errors.New("always fails")
	calls := 0
	err := policy.Do(context.Background(), func() error {
		calls++
		return retriableErr
	}, func(error) bool { return true })

	if !errors.Is(err, retriableErr) {
		t.Fatalf("Do() error = %v, want %v", err, retriableErr)
	}
	// Основной вызов + len(Delays) повторов.
	wantCalls := 1 + len(policy.Delays)
	if calls != wantCalls {
		t.Errorf("calls = %d, want %d", calls, wantCalls)
	}
}

func TestDo_EmptyPolicyDoesNotRetry(t *testing.T) {
	t.Parallel()

	// Пустая политика (без Delays) => ровно одна попытка, даже если ошибка
	// retriable - так пользуется финальная отправка агента при shutdown.
	retriableErr := errors.New("temporary")
	calls := 0
	err := Policy{}.Do(context.Background(), func() error {
		calls++
		return retriableErr
	}, func(error) bool { return true })

	if !errors.Is(err, retriableErr) {
		t.Fatalf("Do() error = %v, want %v", err, retriableErr)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (empty policy = single attempt)", calls)
	}
}

func TestDo_ContextCancelInterruptsWait(t *testing.T) {
	t.Parallel()

	// Delays намеренно реальные: тест проверяет, что отмена контекста прерывает
	// ожидание раньше полного delay, а не то, что delay короткий.
	policy := DefaultPolicy()
	ctx, cancel := context.WithCancel(context.Background())

	retriableErr := errors.New("temporary")
	calls := 0

	done := make(chan error, 1)
	go func() {
		done <- policy.Do(ctx, func() error {
			calls++
			return retriableErr
		}, func(error) bool { return true })
	}()

	// Даём Do дойти до ожидания после первого вызова, затем отменяем контекст -
	// если select с ctx.Done() работает, Do вернётся почти сразу, а не через Delays[0]=1s.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Do() error = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Do() did not return promptly after ctx cancellation")
	}

	if calls != 1 {
		t.Errorf("calls = %d, want 1 (cancelled during first wait)", calls)
	}
}
