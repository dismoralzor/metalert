package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dismoralzor/metalert/internal/repository"
)

func TestUpdatesJSONHandler_Update(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{
			name:     "valid batch",
			body:     `[{"id":"Temperature","type":"gauge","value":23.5},{"id":"Requests","type":"counter","delta":5}]`,
			wantCode: http.StatusOK,
		},
		{
			name:     "empty batch",
			body:     `[]`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "malformed json",
			body:     `[{"id":"X","type":`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "not an array",
			body:     `{"id":"X","type":"gauge","value":1}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "one metric in batch invalid - whole batch rejected",
			body:     `[{"id":"Temperature","type":"gauge","value":23.5},{"id":"X","type":"counter"}]`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unknown metric type",
			body:     `[{"id":"X","type":"histogram","value":1}]`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty id",
			body:     `[{"id":"","type":"gauge","value":1}]`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewUpdatesJSONHandler(repository.NewMemStorage())

			req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			h.Update(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("Update() code = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

// Батч должен реально попадать в хранилище, а не только отвечать 200.
func TestUpdatesJSONHandler_StoresMetrics(t *testing.T) {
	storage := repository.NewMemStorage()
	h := NewUpdatesJSONHandler(storage)

	body := `[{"id":"Temperature","type":"gauge","value":23.5},{"id":"Requests","type":"counter","delta":5}]`
	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Update(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Update() code = %d, want %d", w.Code, http.StatusOK)
	}

	if value, ok := storage.GetGauge("Temperature"); !ok || value != 23.5 {
		t.Errorf("GetGauge(Temperature) = %v, %v, want 23.5, true", value, ok)
	}
	if delta, ok := storage.GetCounter("Requests"); !ok || delta != 5 {
		t.Errorf("GetCounter(Requests) = %v, %v, want 5, true", delta, ok)
	}
}

// Дубли counter внутри одного батча должны сложиться, а не перезаписаться.
func TestUpdatesJSONHandler_DuplicateCountersAccumulate(t *testing.T) {
	storage := repository.NewMemStorage()
	h := NewUpdatesJSONHandler(storage)

	body := `[{"id":"Requests","type":"counter","delta":5},{"id":"Requests","type":"counter","delta":3}]`
	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	w := httptest.NewRecorder()

	h.Update(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Update() code = %d, want %d", w.Code, http.StatusOK)
	}

	if delta, ok := storage.GetCounter("Requests"); !ok || delta != 8 {
		t.Errorf("GetCounter(Requests) = %v, %v, want 8, true", delta, ok)
	}
}
