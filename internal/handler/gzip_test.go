package handler

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dismoralzor/metalert/internal/repository"
)

func TestGzipMiddleware(t *testing.T) {
	srv := httptest.NewServer(
		GzipMiddleware(http.HandlerFunc(
			NewUpdateJSONHandler(repository.NewMemStorage()).Update,
		)),
	)
	defer srv.Close()

	const requestBody = `{"id":"Temperature","type":"gauge","value":23.5}`
	const wantBody = `{"id":"Temperature","type":"gauge","value":23.5}`

	// DisableCompression, иначе транспорт сам добавит Accept-Encoding и сам же
	// распакует ответ - тогда accepts_gzip проверял бы Go, а не наш middleware.
	client := &http.Client{
		Transport: &http.Transport{DisableCompression: true},
	}

	t.Run("sends_gzip", func(t *testing.T) {
		var compressed bytes.Buffer
		zw := gzip.NewWriter(&compressed)
		if _, err := zw.Write([]byte(requestBody)); err != nil {
			t.Fatalf("compress request: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("close gzip writer: %v", err)
		}

		req, err := http.NewRequest(http.MethodPost, srv.URL, &compressed)
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Content-Encoding", "gzip")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		// Accept-Encoding не отправляли, значит ответ должен прийти несжатым.
		if enc := resp.Header.Get("Content-Encoding"); enc != "" {
			t.Errorf("Content-Encoding = %q, want empty", enc)
		}
		assertJSONBody(t, resp.Body, wantBody)
	})

	t.Run("accepts_gzip", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(requestBody))
		if err != nil {
			t.Fatalf("build request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept-Encoding", "gzip")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if enc := resp.Header.Get("Content-Encoding"); enc != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", enc)
		}

		zr, err := gzip.NewReader(resp.Body)
		if err != nil {
			t.Fatalf("gzip reader: %v", err)
		}
		defer zr.Close()

		assertJSONBody(t, zr, wantBody)
	})
}
