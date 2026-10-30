// Package retry содержит общую политику повторов для сбойных операций:
// БД (internal/repository) и HTTP-отправку метрик (cmd/agent).
package retry

import (
	"context"
	"time"
)

// Policy описывает политику повторов: паузы между попытками. Длина Delays - это
// число ДОПОЛНИТЕЛЬНЫХ попыток поверх основной (пустой Delays => ровно одна
// попытка, без повторов). Передаётся как зависимость, а не хранится глобально:
// прод берёт DefaultPolicy(), тесты - свою с миллисекундными паузами.
type Policy struct {
	Delays []time.Duration
}

// DefaultPolicy возвращает рабочую политику: паузы [1s, 3s, 5s], то есть до 3
// дополнительных попыток поверх основной (максимум 4 обращения). Каждый вызов
// возвращает НОВЫЙ срез - вызывающие стороны не делят общий изменяемый слайс.
func DefaultPolicy() Policy {
	return Policy{Delays: []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}}
}

// Do выполняет fn. При успехе - nil сразу. При ошибке, для которой isRetriable
// вернул false, - ошибка возвращается немедленно, без пауз (незачем повторять то,
// что гарантированно провалится снова). При retriable-ошибке ждём p.Delays[attempt]
// и пробуем снова; после исчерпания Delays возвращаем последнюю ошибку.
//
// Пауза сделана через select с ctx.Done(), а не через time.Sleep: если вызывающая
// сторона отменит контекст (например, graceful shutdown), ожидание прервётся сразу,
// а не провисит до 5 секунд впустую.
func (p Policy) Do(ctx context.Context, fn func() error, isRetriable func(error) bool) error {
	var err error

	for attempt := 0; ; attempt++ {
		err = fn()
		if err == nil {
			return nil
		}
		if !isRetriable(err) {
			return err
		}
		if attempt >= len(p.Delays) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(p.Delays[attempt]):
		}
	}
}
