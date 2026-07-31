package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dismoralzor/metalert/internal/repository"
)

func TestValueJSONHandler_Value(t *testing.T) {
	// Одно хранилище на все подтесты: Value только читает.
	storage := repository.NewMemStorage()
	storage.UpdateGauge("Temperature", 23.5)
	storage.UpdateCounter("Requests", 10)
	h := NewValueJSONHandler(storage)

	tests := []struct {
		name     string
		body     string
		wantCode int
		wantBody string
	}{
		{
			name:     "existing gauge",
			body:     `{"id":"Temperature","type":"gauge"}`,
			wantCode: http.StatusOK,
			wantBody: `{"id":"Temperature","type":"gauge","value":23.5}`,
		},
		{
			name:     "existing counter",
			body:     `{"id":"Requests","type":"counter"}`,
			wantCode: http.StatusOK,
			wantBody: `{"id":"Requests","type":"counter","delta":10}`,
		},
		{
			name:     "unknown metric name",
			body:     `{"id":"Missing","type":"gauge"}`,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "unknown metric type",
			body:     `{"id":"Temperature","type":"histogram"}`,
			wantCode: http.StatusNotFound,
		},
		{
			name:     "malformed json",
			body:     `{"id":"Temperature","type":`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/value", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			h.Value(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("Value() code = %d, want %d", w.Code, tt.wantCode)
			}
			if tt.wantCode != http.StatusOK {
				return
			}

			if ct := w.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			assertJSONBody(t, w.Result().Body, tt.wantBody)
		})
	}
}
