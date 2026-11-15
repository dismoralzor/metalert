package main

import (
	"context"

	models "github.com/dismoralzor/metalert/internal/model"
)

// sendFunc - сигнатура фактической отправки батча; в проде это sender.Send,
// в тестах воркер-пула подменяется фейком, чтобы не ходить в сеть и управлять
// таймингом/конкурентностью напрямую.
type sendFunc func(ctx context.Context, metrics []models.Metrics) bool

// runWorker - один воркер из пула: читает задания из jobsCh и отправляет их send'ом
// (gzip + подпись + retry - вся эта логика уже внутри sender, здесь не дублируется).
// Завершается и по закрытию jobsCh (работа кончилась), и по ctx.Done() (shutdown),
// смотря что наступит раньше.
func runWorker(
	ctx context.Context,
	jobsCh <-chan job,
	resultsCh chan<- sendResult,
	send sendFunc,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case j, ok := <-jobsCh:
			if !ok {
				return
			}

			success := send(ctx, j.metrics)

			select {
			case resultsCh <- sendResult{pollDelta: j.pollDelta, success: success}:
			case <-ctx.Done():
			}
		}
	}
}
