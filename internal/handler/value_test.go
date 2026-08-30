package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dismoralzor/metalert/internal/repository"
)

func TestValueHandler_Value(t *testing.T) {
	// Один storage на все подтесты: Value только читает, изоляция не нужна.
	storage := repository.NewMemStorage()
	storage.UpdateGauge(context.Background(), "temperature", 23.5)
	storage.UpdateCounter(context.Background(), "requests", 10)
	h := NewValueHandler(storage)

	tests := []struct {
		name       string
		metricType string
		metricName string
		wantCode   int
		wantBody   string
	}{
		{
			name:       "existing gauge",
			metricType: "gauge",
			metricName: "temperature",
			wantCode:   http.StatusOK,
			wantBody:   "23.5",
		},
		{
			name:       "existing counter",
			metricType: "counter",
			metricName: "requests",
			wantCode:   http.StatusOK,
			wantBody:   "10",
		},
		{
			name:       "unknown metric name",
			metricType: "gauge",
			metricName: "unknown",
			wantCode:   http.StatusNotFound,
		},
		{
			name:       "unknown metric type",
			metricType: "unknown",
			metricName: "temperature",
			wantCode:   http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/value/"+tt.metricType+"/"+tt.metricName, nil)
			req = withChiURLParams(req, map[string]string{
				"type": tt.metricType,
				"name": tt.metricName,
			})

			w := httptest.NewRecorder()
			h.Value(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("Value() code = %d, want %d", w.Code, tt.wantCode)
			}

			if tt.wantCode == http.StatusOK {
				body, err := io.ReadAll(w.Result().Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				if string(body) != tt.wantBody {
					t.Errorf("Value() body = %q, want %q", body, tt.wantBody)
				}
			}
		})
	}
}
