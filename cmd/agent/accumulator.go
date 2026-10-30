package main

import (
	"context"
	"log"
	"time"

	models "github.com/dismoralzor/metalert/internal/model"
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
//
// При ctx.Done() накопленное НЕ выбрасывается: если есть pending или ненулевой
// pollCount, аккумулятор собирает финальный батч и делает одну попытку отправки
// через sender.SendOnce (без retry) под отдельным коротким контекстом - основной
// ctx уже отменён, поэтому его нельзя использовать для сетевого вызова. Ошибку
// финальной отправки только логируем: процесс всё равно завершается.
func runAccumulator(
	ctx context.Context,
	reportInterval time.Duration,
	metricsCh <-chan models.Metrics,
	jobsCh chan<- job,
	resultsCh <-chan sendResult,
	a *agent,
	s *sender,
) {
	defer close(jobsCh)

	ticker := time.NewTicker(reportInterval)
	defer ticker.Stop()

	var pending []models.Metrics

	for {
		select {
		case <-ctx.Done():
			flushFinal(ctx, pending, a, s)
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

// flushFinal отправляет накопленное при остановке агента. Делает это, только если
// есть что отправлять (pending непустой или pollCount > 0), одной попыткой без
// retry и под собственным коротким контекстом, т.к. переданный ctx уже отменён.
func flushFinal(ctx context.Context, pending []models.Metrics, a *agent, s *sender) {
	delta := a.pollCount.Swap(0)
	if len(pending) == 0 && delta == 0 {
		return
	}
	if s == nil {
		return
	}

	d := delta
	batch := append(pending, models.Metrics{ID: "PollCount", MType: models.Counter, Delta: &d})

	// Отдельный контекст: основной ctx уже Done, но финальную отправку хочется
	// успеть сделать. WithoutCancel отвязывает от отмены родителя, таймаут
	// ограничивает ожидание.
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()

	if err := s.SendOnce(sendCtx, batch); err != nil {
		log.Printf("final send on shutdown: %v", err)
	}
}
