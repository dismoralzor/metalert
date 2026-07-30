package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/dismoralzor/metalert/internal/repository"
)

// Сравнивает как разобранный JSON, а не как строки: иначе тест ломался бы
// от порядка полей и от перевода строки, который добавляет json.Encoder.
func assertJSONBody(t *testing.T, got io.Reader, want string) {
	t.Helper()

	var gotVal, wantVal any
	if err := json.NewDecoder(got).Decode(&gotVal); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		t.Fatalf("decode want body: %v", err)
	}

	if !reflect.DeepEqual(gotVal, wantVal) {
		t.Errorf("body = %#v, want %#v", gotVal, wantVal)
	}
}

func TestUpdateJSONHandler_Update(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
		wantBody string
	}{
		{
			name:     "valid gauge",
			body:     `{"id":"Temperature","type":"gauge","value":23.5}`,
			wantCode: http.StatusOK,
			wantBody: `{"id":"Temperature","type":"gauge","value":23.5}`,
		},
		{
			name:     "valid counter",
			body:     `{"id":"Requests","type":"counter","delta":5}`,
			wantCode: http.StatusOK,
			wantBody: `{"id":"Requests","type":"counter","delta":5}`,
		},
		{
			name:     "malformed json",
			body:     `{"id":"X","type":`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "gauge without value",
			body:     `{"id":"X","type":"gauge"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "counter without delta",
			body:     `{"id":"X","type":"counter"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unknown metric type",
			body:     `{"id":"X","type":"histogram","value":1}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "empty id",
			body:     `{"id":"","type":"gauge","value":1}`,
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Своё хранилище на подтест: counter накапливается.
			h := NewUpdateJSONHandler(repository.NewMemStorage())

			req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(tt.body))
			w := httptest.NewRecorder()

			h.Update(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("Update() code = %d, want %d", w.Code, tt.wantCode)
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

// Ответ должен содержать накопленное значение, а не присланную delta.
func TestUpdateJSONHandler_CounterAccumulates(t *testing.T) {
	h := NewUpdateJSONHandler(repository.NewMemStorage())

	var lastBody io.Reader
	for range 2 {
		req := httptest.NewRequest(http.MethodPost, "/update",
			strings.NewReader(`{"id":"Cnt","type":"counter","delta":5}`))
		w := httptest.NewRecorder()

		h.Update(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("Update() code = %d, want %d", w.Code, http.StatusOK)
		}
		lastBody = w.Result().Body
	}

	assertJSONBody(t, lastBody, `{"id":"Cnt","type":"counter","delta":10}`)
}
