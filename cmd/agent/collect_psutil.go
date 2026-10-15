package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"

	"github.com/dismoralzor/metalert/internal/model"
)

// collectPSUtilMetrics - горутина B: собирает системные метрики через gopsutil
// каждые pollInterval, независимо от горутины A (свой отдельный тикер).
func collectPSUtilMetrics(ctx context.Context, pollInterval time.Duration, metricsCh chan<- models.Metrics) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if vm, err := mem.VirtualMemory(); err != nil {
				log.Printf("gopsutil: read virtual memory: %v", err)
			} else {
				total, free := float64(vm.Total), float64(vm.Free)
				sendMetric(ctx, metricsCh, models.Metrics{ID: "TotalMemory", MType: models.Gauge, Value: &total})
				sendMetric(ctx, metricsCh, models.Metrics{ID: "FreeMemory", MType: models.Gauge, Value: &free})
			}

			// interval=pollInterval (а не 0): второй аргумент cpu.Percent просит
			// gopsutil самому измерить загрузку за этот промежуток, без него первый
			// вызов вернул бы 0 (нет предыдущей точки для сравнения). Блокирует
			// эту горутину ещё на pollInterval внутри самого cpu.Percent - реальный
			// период сэмплирования CPU получается около 2×pollInterval, но это
			// не мешает работе A и остального пайплайна (у каждого свой тикер).
			percentages, err := cpu.Percent(pollInterval, true)
			if err != nil {
				log.Printf("gopsutil: read cpu percent: %v", err)
				continue
			}
			for _, m := range buildCPUMetrics(percentages) {
				sendMetric(ctx, metricsCh, m)
			}
		}
	}
}

// buildCPUMetrics превращает cpu.Percent(..., true) в метрики CPUutilization1..N -
// количество и имена определяются В РАНТАЙМЕ по длине percentages (числу логических
// CPU), а не захардкожены. Вынесено отдельно от collectPSUtilMetrics, чтобы
// проверять генерацию имён без реального сбора данных gopsutil в тестах.
func buildCPUMetrics(percentages []float64) []models.Metrics {
	metrics := make([]models.Metrics, 0, len(percentages))
	for i, p := range percentages {
		v := p
		metrics = append(metrics, models.Metrics{
			ID:    fmt.Sprintf("CPUutilization%d", i+1),
			MType: models.Gauge,
			Value: &v,
		})
	}
	return metrics
}
