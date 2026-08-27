package handler

import (
	"bytes"
	"io"
	"net/http"

	"github.com/dismoralzor/metalert/internal/hash"
)

// hashResponseWriter буферизует всё тело ответа и считает его подпись ПЕРЕД
// тем, как отдать что-либо реальному ResponseWriter - заголовки нельзя менять
// после первого Write, поэтому HashSHA256 нужно выставить раньше него.
type hashResponseWriter struct {
	http.ResponseWriter
	key        string
	buf        bytes.Buffer
	statusCode int
}

func newHashResponseWriter(w http.ResponseWriter, key string) *hashResponseWriter {
	return &hashResponseWriter{ResponseWriter: w, key: key, statusCode: http.StatusOK}
}

// WriteHeader запоминает код ответа, но НЕ пробрасывает его дальше - реальный
// WriteHeader вызовется только из Flush, когда тело уже целиком в буфере.
func (h *hashResponseWriter) WriteHeader(statusCode int) {
	h.statusCode = statusCode
}

func (h *hashResponseWriter) Write(b []byte) (int, error) {
	return h.buf.Write(b)
}

// Flush считает подпись накопленного тела, выставляет заголовок и только
// теперь пишет статус и тело в настоящий ResponseWriter. Вызывается один раз,
// после того как хендлер полностью отработал.
func (h *hashResponseWriter) Flush() {
	sum := hash.Compute(h.buf.Bytes(), h.key)
	h.ResponseWriter.Header().Set("HashSHA256", sum)
	h.ResponseWriter.WriteHeader(h.statusCode)
	h.ResponseWriter.Write(h.buf.Bytes())
}

// HashMiddleware проверяет подпись тела запроса и подписывает тело ответа -
// но только если ключ задан, иначе полностью прозрачен (как было до инкремента).
//
// ПОРЯДОК ВАЖЕН: должен подключаться ПОСЛЕ GzipMiddleware (r.Use(GzipMiddleware),
// затем r.Use(HashMiddleware(key))) - агент считает хеш от НЕсжатого тела,
// значит и сервер обязан проверять подпись уже после распаковки gzip.
func HashMiddleware(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Заголовка может не быть - не все запросы подписаны (например,
			// автотесты без ключа на стороне агента), пропускаем как есть.
			if got := r.Header.Get("HashSHA256"); got != "" {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "failed to read body", http.StatusBadRequest)
					return
				}
				r.Body.Close()

				if !hash.Valid(body, key, got) {
					http.Error(w, "invalid hash signature", http.StatusBadRequest)
					return
				}

				// io.ReadAll уже вычитал тело - без восстановления хендлер
				// увидел бы пустой r.Body.
				r.Body = io.NopCloser(bytes.NewReader(body))
			}

			hw := newHashResponseWriter(w, key)
			next.ServeHTTP(hw, r)
			hw.Flush()
		})
	}
}
