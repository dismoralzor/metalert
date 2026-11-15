package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dismoralzor/metalert/internal/repository"
)

func TestIndexHandler_Index(t *testing.T) {
	storage := repository.NewMemStorage()
	storage.UpdateGauge(context.Background(), "temperature", 23.5)
	storage.UpdateCounter(context.Background(), "requests", 10)
	h := NewIndexHandler(storage)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.Index(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Index() code = %d, want %d", w.Code, http.StatusOK)
	}

	body, err := io.ReadAll(w.Result().Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	// Порядок в map рандомизирован, поэтому проверяем подстроки, а не всю страницу целиком.
	for _, want := range []string{"temperature", "23.5", "requests", "10"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("Index() body missing %q; body = %s", want, body)
		}
	}
}
