package handler_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/dismoralzor/metalert/internal/crypto"
	"github.com/dismoralzor/metalert/internal/handler"
	"github.com/dismoralzor/metalert/internal/hash"
	models "github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/repository"
)

// chain собирает цепочку middleware ровно как сервер: расшифровка -> gzip ->
// HMAC -> хендлер батча.
func chain(priv *rsa.PrivateKey, hmacKey string, storage repository.Storage) http.Handler {
	r := chi.NewRouter()
	r.Use(handler.DecryptMiddleware(priv))
	r.Use(handler.GzipMiddleware)
	r.Use(handler.HashMiddleware(hmacKey))
	r.Post("/updates/", handler.NewUpdatesJSONHandler(storage, nil).Update)
	return r
}

// innerBody готовит "внутреннее" тело как агент: JSON -> gzip, плюс HMAC от
// сырого JSON (если ключ задан).
func innerBody(t *testing.T, metrics []models.Metrics, hmacKey string) (body []byte, sig string) {
	t.Helper()
	raw, err := json.Marshal(metrics)
	if err != nil {
		t.Fatal(err)
	}
	if hmacKey != "" {
		sig = hash.Compute(raw, hmacKey)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes(), sig
}

func gaugeBatch(id string, v float64) []models.Metrics {
	return []models.Metrics{{ID: id, MType: models.Gauge, Value: &v}}
}

func TestDecryptMiddleware_EncryptedRequest(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	const hmacKey = "secret"
	storage := repository.NewMemStorage()
	h := chain(key, hmacKey, storage)

	inner, sig := innerBody(t, gaugeBatch("Alloc", 1.5), hmacKey)
	encrypted, err := crypto.Encrypt(&key.PublicKey, inner)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(encrypted))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("HashSHA256", sig)
	req.Header.Set("X-Content-Encrypted", "true")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("код = %d, want 200 (body=%q)", w.Code, w.Body.String())
	}
	if v, ok := storage.GetGauge(context.Background(), "Alloc"); !ok || v != 1.5 {
		t.Errorf("метрика не сохранена: v=%v ok=%v", v, ok)
	}
}

func TestDecryptMiddleware_PlainRequestWithKeyLoaded(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	storage := repository.NewMemStorage()
	h := chain(key, "", storage) // ключ загружен, но запрос НЕ шифрован

	inner, _ := innerBody(t, gaugeBatch("Sys", 2.5), "")
	req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(inner))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	// X-Content-Encrypted НЕ ставим.

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("код = %d, want 200 (незашифрованный запрос при загруженном ключе)", w.Code)
	}
	if v, ok := storage.GetGauge(context.Background(), "Sys"); !ok || v != 2.5 {
		t.Errorf("метрика не сохранена: v=%v ok=%v", v, ok)
	}
}

func TestDecryptMiddleware_EncryptedWithoutKey(t *testing.T) {
	storage := repository.NewMemStorage()
	h := chain(nil, "", storage) // ключа нет

	req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader([]byte("anything")))
	req.Header.Set("X-Content-Encrypted", "true")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("код = %d, want 400 (зашифровано, а ключа нет)", w.Code)
	}
}

func TestDecryptMiddleware_CorruptedBody(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	storage := repository.NewMemStorage()
	h := chain(key, "", storage)

	req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader([]byte("не валидный payload")))
	req.Header.Set("X-Content-Encrypted", "true")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("код = %d, want 400 (битое зашифрованное тело)", w.Code)
	}
}
