package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dismoralzor/metalert/internal/hash"
)

// echoHandler читает тело запроса целиком и отправляет его же в ответе -
// удобно проверять и восстановление r.Body, и подпись ответа.
func echoHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	})
}

func TestHashMiddleware_ValidSignature(t *testing.T) {
	key := "secret"
	body := `[{"id":"x","type":"gauge","value":1}]`
	sig := hash.Compute([]byte(body), key)

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	req.Header.Set("HashSHA256", sig)
	w := httptest.NewRecorder()

	HashMiddleware(key)(echoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	// Хендлер должен был увидеть полное тело, а не пустое (r.Body восстановлен
	// после io.ReadAll в middleware).
	if got := w.Body.String(); got != body {
		t.Errorf("handler saw body = %q, want %q", got, body)
	}
}

func TestHashMiddleware_InvalidSignature(t *testing.T) {
	key := "secret"
	body := `[{"id":"x","type":"gauge","value":1}]`

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	req.Header.Set("HashSHA256", "0000000000000000000000000000000000000000000000000000000000000000")
	w := httptest.NewRecorder()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })

	HashMiddleware(key)(next).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if called {
		t.Error("next handler was called despite invalid signature")
	}
}

func TestHashMiddleware_NoKeyConfigured_PassesThrough(t *testing.T) {
	body := `[{"id":"x","type":"gauge","value":1}]`

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	// Заголовок пришёл, но ключ не сконфигурирован - проверять нечем и незачем.
	req.Header.Set("HashSHA256", "garbage")
	w := httptest.NewRecorder()

	HashMiddleware("")(echoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
	// Без ключа обёртка ответа не применяется - заголовка подписи быть не должно.
	if got := w.Header().Get("HashSHA256"); got != "" {
		t.Errorf("response HashSHA256 = %q, want empty (no key configured)", got)
	}
}

func TestHashMiddleware_NoHeaderPresent_PassesThrough(t *testing.T) {
	key := "secret"
	body := `[{"id":"x","type":"gauge","value":1}]`

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	// Ключ задан, но конкретный запрос не подписан - не все запросы подписаны.
	w := httptest.NewRecorder()

	HashMiddleware(key)(echoHandler()).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Body.String(); got != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestHashMiddleware_SignsResponseWhenKeyConfigured(t *testing.T) {
	key := "secret"
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	respBody := "response payload"
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(respBody))
	})

	HashMiddleware(key)(next).ServeHTTP(w, req)

	want := hash.Compute([]byte(respBody), key)
	if got := w.Header().Get("HashSHA256"); got != want {
		t.Errorf("response HashSHA256 = %q, want %q", got, want)
	}
	if w.Body.String() != respBody {
		t.Errorf("response body = %q, want %q", w.Body.String(), respBody)
	}
}
