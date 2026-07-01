package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dismoralzor/metalert/internal/repository"
)

func TestUpdateHandler_Update(t *testing.T) {
	tests := []struct {
		name       string
		metricType string
		metricName string
		value      string
		wantCode   int
	}{
		{
			name:       "valid gauge",
			metricType: "gauge",
			metricName: "temperature",
			value:      "23.5",
			wantCode:   http.StatusOK,
		},
		{
			name:       "valid counter",
			metricType: "counter",
			metricName: "requests",
			value:      "10",
			wantCode:   http.StatusOK,
		},
		{
			name:       "unknown metric type",
			metricType: "unknown",
			metricName: "foo",
			value:      "1",
			wantCode:   http.StatusBadRequest,
		},
		{
			name:       "invalid gauge value",
			metricType: "gauge",
			metricName: "temperature",
			value:      "not-a-number",
			wantCode:   http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := repository.NewMemStorage()
			h := NewUpdateHandler(storage)

			req := httptest.NewRequest(http.MethodPost, "/update/"+tt.metricType+"/"+tt.metricName+"/"+tt.value, nil)
			// SetPathValue имитирует то, что при реальном запросе делает http.ServeMux
			// при сопоставлении с паттерном "{type}/{name}/{value}".
			req.SetPathValue("type", tt.metricType)
			req.SetPathValue("name", tt.metricName)
			req.SetPathValue("value", tt.value)

			w := httptest.NewRecorder()
			h.Update(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("Update() code = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}
