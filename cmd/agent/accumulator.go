package main

import (
	"context"
	"time"

	"github.com/dismoralzor/metalert/internal/model"
)

// job - задание для воркера: готовый батч плюс тот кусок pollCount, за который
// это задание "отвечает" (нужно для восстановления счётчика при неудаче).
type job struct {
	metrics   []models.Metrics
	pollDelta int64
}

// sendResult - ответ воркера аккумулятору: удалось ли отправить батч job.
type sendResult struct {
	pollDelta int64
	success   bool
}

// runAccumulator - горутина-аккумулятор: копит метрики из metricsCh, по каждому
// тику reportInterval формирует из накопленного батч и кладёт задание в jobsCh.
// Закрывает jobsCh при выходе - воркеры используют это как сигнал "работы больше
// не будет" в дополнение к ctx.Done().
//
// pollCount при асинхронной отправке: аккумулятор ЗАБИРАЕТ (обнуляет) текущее
// значение pollCount в момент формирования батча - это "оптимистичное" вычитание,
// а не ожидание подтверждения. Такой кусок принадлежит ровно этому job и не
// пересекается с другими одновременно летящими батчами (в отличие от подхода
// "вычесть после ack", который дал бы двойной счёт при перекрывающихся отправках
// через пул воркеров). Если отправка проваливается, воркер вернёт результат с
// success=false через resultsCh, и аккумулятор ВОЗВРАЩАЕТ delta обратно в
// pollCount - он попадёт в следующий батч вместе с новыми накопленными опросами,
// а не потеряется.
func runAccumulator(
	ctx context.Context,
	reportInterval time.Duration,
	metricsCh <-chan models.Metrics,
	jobsCh chan<- job,
	resultsCh <-chan sendResult,
	a *agent,
) {
	defer close(jobsCh)

	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	var pending []models.Metrics

	for {
		select {
		case <-ctx.Done():
			return

		case m := <-metricsCh:
			pending = append(pending, m)

		case res := <-resultsCh:
			if !res.success {
				a.pollCount.Add(res.pollDelta)
			}

		case <-ticker.C:
			if len(pending) == 0 {
				continue
			}

			delta := a.pollCount.Swap(0)

			d := delta
			batch := append(pending, models.Metrics{ID: "PollCount", MType: models.Counter, Delta: &d})
			pending = nil

			select {
			case jobsCh <- job{metrics: batch, pollDelta: delta}:
			case <-ctx.Done():
				return
			}
		}
	}
}
