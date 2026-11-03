package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/dismoralzor/metalert/internal/crypto"
	models "github.com/dismoralzor/metalert/internal/model"
)

// При падении первой попытки ретрай обязан отправить ПОЛНОЕ зашифрованное тело,
// а не пустое (тело пересобирается новым Reader'ом на каждую попытку).
func TestSender_RetrySendsFullEncryptedBody(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var attempts int32
	var lastPlain []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)

		if r.Header.Get("X-Content-Encrypted") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payload, _ := io.ReadAll(r.Body)

		// Первая попытка падает (5xx) - должен сработать retry.
		if n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		plain, derr := crypto.Decrypt(key, payload)
		if derr != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		lastPlain = plain
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	v := 1.5
	batch := []models.Metrics{{ID: "Alloc", MType: models.Gauge, Value: &v}}

	s := newSender(srv.Client(), srv.Listener.Addr().String(), "", &key.PublicKey, shortPolicy())
	if !s.Send(context.Background(), batch) {
		t.Fatal("Send() = false, ожидался успех после ретрая")
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Fatalf("попыток = %d, ожидалось 2 (первая 500, вторая 200)", got)
	}

	// Тело второй попытки должно быть полным: расшифровывается и распаковывается
	// в исходный батч, а не в пустоту.
	if len(lastPlain) == 0 {
		t.Fatal("тело повторной попытки пустое")
	}
	zr, err := gzip.NewReader(bytes.NewReader(lastPlain))
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("распаковка: %v", err)
	}
	var got []models.Metrics
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got) != 1 || got[0].ID != "Alloc" {
		t.Errorf("расшифрованный батч = %+v, ожидался [Alloc]", got)
	}
}
