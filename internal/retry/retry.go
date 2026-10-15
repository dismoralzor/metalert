// Package retry содержит общую политику повторов для сбойных операций:
// БД (internal/repository) и HTTP-отправку метрик (cmd/agent).
package retry

import (
	"context"
	"time"
)

// Delays - паузы между повторами: [1s, 3s, 5s] по заданию, то есть 3 ДОПОЛНИТЕЛЬНЫЕ
// попытки поверх основной (максимум 4 обращения всего). Var, а не const - тесты
// временно уменьшают его, чтобы не ждать реальные секунды.
var Delays = []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}

// Do выполняет fn. При успехе - nil сразу. При ошибке, для которой isRetriable
// вернул false, - ошибка возвращается немедленно, без пауз (незачем повторять то,
// что гарантированно провалится снова). При retriable-ошибке ждём Delays[attempt]
// и пробуем снова; после исчерпания Delays возвращаем последнюю ошибку.
//
// Пауза сделана через select с ctx.Done(), а не через time.Sleep: если вызывающая
// сторона отменит контекст (например, graceful shutdown), ожидание прервётся сразу,
// а не провисит до 5 секунд впустую.
func Do(ctx context.Context, fn func() error, isRetriable func(error) bool) error {
	var err error

	for attempt := 0; ; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isRetriable(err) {
			return err
		}
		if attempt >= len(Delays) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(Delays[attempt]):
		}
	}
}
