package main

import (
	"context"

	"github.com/dismoralzor/metalert/internal/model"
)

// sendMetric кладёт метрику в канал, но не блокируется навечно, если получатель
// уже остановился при отмене контекста - иначе горутина-отправитель зависла бы
// на закрытии приложения.
func sendMetric(ctx context.Context, ch chan<- models.Metrics, m models.Metrics) {
	select {
	case ch <- m:
	case <-ctx.Done():
	}
}
