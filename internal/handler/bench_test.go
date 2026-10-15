package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// benchStorage наполняет хранилище реалистичным набором: n gauge + n counter.
func benchStorage(n int) repository.Storage {
	s := repository.NewMemStorage()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		s.UpdateGauge(ctx, fmt.Sprintf("gauge_%d", i), float64(i)+0.5)
		s.UpdateCounter(ctx, fmt.Sprintf("counter_%d", i), int64(i))
	}
	return s
}

// BenchmarkIndexHandler - рендер HTML-страницы со всеми метриками: самый
// аллоцирующий путь (построение DTO, разбиение на строки таблицы,
// форматирование значений и рендер шаблона в буфер).
func BenchmarkIndexHandler(b *testing.B) {
	h := NewIndexHandler(benchStorage(500))
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		h.Index(w, req)
	}
}

// BenchmarkUpdatesBatch - приём батча метрик: JSON-декод тела + запись батча.
func BenchmarkUpdatesBatch(b *testing.B) {
	const n = 300
	metrics := make([]models.Metrics, 0, n)
	for i := 0; i < n; i++ {
		v := float64(i) + 0.5
		metrics = append(metrics, models.Metrics{ID: fmt.Sprintf("gauge_%d", i), MType: models.Gauge, Value: &v})
	}
	body, err := json.Marshal(metrics)
	if err != nil {
		b.Fatal(err)
	}

	h := NewUpdatesJSONHandler(repository.NewMemStorage(), nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/updates", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.Update(w, req)
	}
}
