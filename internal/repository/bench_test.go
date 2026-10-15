package repository

import (
	"context"
	"fmt"
	"testing"

	models "github.com/dismoralzor/metalert/internal/model"
)

// benchBatch готовит реалистичный батч: n gauge + n counter.
func benchBatch(n int) []models.Metrics {
	metrics := make([]models.Metrics, 0, 2*n)
	for i := 0; i < n; i++ {
		v := float64(i) + 0.5
		d := int64(i)
		metrics = append(metrics,
			models.Metrics{ID: fmt.Sprintf("gauge_%d", i), MType: models.Gauge, Value: &v},
			models.Metrics{ID: fmt.Sprintf("counter_%d", i), MType: models.Counter, Delta: &d},
		)
	}
	return metrics
}

func BenchmarkUpdateBatch(b *testing.B) {
	batch := benchBatch(150)
	s := NewMemStorage()
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := s.UpdateBatch(ctx, batch); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMetrics(b *testing.B) {
	s := NewMemStorage()
	ctx := context.Background()
	for i := 0; i < 500; i++ {
		s.UpdateGauge(ctx, fmt.Sprintf("gauge_%d", i), float64(i)+0.5)
		s.UpdateCounter(ctx, fmt.Sprintf("counter_%d", i), int64(i))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.Metrics(ctx)
	}
}
