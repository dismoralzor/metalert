package main

import (
	"context"
	"math/rand"
	"runtime"
	"time"

	"github.com/dismoralzor/metalert/internal/model"
)

// collectRuntimeMetrics - горутина A: собирает runtime.MemStats + RandomValue
// каждые pollInterval и шлёт их в общий metricsCh, а не копит в структуре -
// накопление теперь на стороне аккумулятора.
//
// pollCount инкрементируется здесь же (под мьютексом a), а не через metricsCh:
// в отличие от обычных gauge-метрик, ему нужна отдельная семантика "сброс только
// после подтверждённой отправки" (см. runAccumulator), которая не укладывается
// в общую схему "аккумулятор слепо копит всё, что пришло по каналу".
func collectRuntimeMetrics(ctx context.Context, pollInterval time.Duration, metricsCh chan<- models.Metrics, a *agent) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			gauges := map[string]float64{
				"Alloc":         float64(m.Alloc),
				"BuckHashSys":   float64(m.BuckHashSys),
				"Frees":         float64(m.Frees),
				"GCCPUFraction": m.GCCPUFraction,
				"GCSys":         float64(m.GCSys),
				"HeapAlloc":     float64(m.HeapAlloc),
				"HeapIdle":      float64(m.HeapIdle),
				"HeapInuse":     float64(m.HeapInuse),
				"HeapObjects":   float64(m.HeapObjects),
				"HeapReleased":  float64(m.HeapReleased),
				"HeapSys":       float64(m.HeapSys),
				"LastGC":        float64(m.LastGC),
				"Lookups":       float64(m.Lookups),
				"MCacheInuse":   float64(m.MCacheInuse),
				"MCacheSys":     float64(m.MCacheSys),
				"MSpanInuse":    float64(m.MSpanInuse),
				"MSpanSys":      float64(m.MSpanSys),
				"Mallocs":       float64(m.Mallocs),
				"NextGC":        float64(m.NextGC),
				"NumForcedGC":   float64(m.NumForcedGC),
				"NumGC":         float64(m.NumGC),
				"OtherSys":      float64(m.OtherSys),
				"PauseTotalNs":  float64(m.PauseTotalNs),
				"StackInuse":    float64(m.StackInuse),
				"StackSys":      float64(m.StackSys),
				"Sys":           float64(m.Sys),
				"TotalAlloc":    float64(m.TotalAlloc),
				// math/rand с Go 1.20+ сеется случайно сам, явный Seed не нужен.
				"RandomValue": rand.Float64(),
			}

			for name, value := range gauges {
				v := value
				sendMetric(ctx, metricsCh, models.Metrics{ID: name, MType: models.Gauge, Value: &v})
			}

			a.pollCount.Add(1)
		}
	}
}
