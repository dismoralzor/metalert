package handler

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

// compressWriter сжимает тело ответа, но только если ответ успешный
// и его Content-Type подлежит сжатию.
type compressWriter struct {
	http.ResponseWriter
	zw          *gzip.Writer
	compress    bool
	wroteHeader bool
}

func newCompressWriter(w http.ResponseWriter) *compressWriter {
	return &compressWriter{
		ResponseWriter: w,
		zw:             gzip.NewWriter(w),
	}
}

func (c *compressWriter) WriteHeader(statusCode int) {
	if c.wroteHeader {
		return
	}
	c.wroteHeader = true

	if statusCode < http.StatusMultipleChoices && compressibleType(c.Header().Get("Content-Type")) {
		c.compress = true
		c.Header().Set("Content-Encoding", "gzip")
		// Длина, посчитанная хендлером, относится к несжатому телу.
		c.Header().Del("Content-Length")
	}

	c.ResponseWriter.WriteHeader(statusCode)
}

func (c *compressWriter) Write(b []byte) (int, error) {
	// Хендлер может писать тело без явного WriteHeader (так делает IndexHandler),
	// а решение о сжатии принимается именно там - принимаем его сами.
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}

	if c.compress {
		return c.zw.Write(b)
	}
	return c.ResponseWriter.Write(b)
}

// Close обязателен: gzip.Writer держит данные в буфере и допишет их только здесь.
func (c *compressWriter) Close() error {
	if !c.compress {
		return nil
	}
	return c.zw.Close()
}

func compressibleType(contentType string) bool {
	return strings.Contains(contentType, "application/json") ||
		strings.Contains(contentType, "text/html")
}

// compressReader распаковывает тело запроса на лету.
type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

func newCompressReader(r io.ReadCloser) (*compressReader, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &compressReader{r: r, zr: zr}, nil
}

func (c *compressReader) Read(p []byte) (int, error) {
	return c.zr.Read(p)
}

func (c *compressReader) Close() error {
	if err := c.r.Close(); err != nil {
		return err
	}
	return c.zr.Close()
}

// GzipMiddleware прозрачно сжимает ответ и распаковывает запрос - хендлеры
// работают с обычными w и r.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ow := w

		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			cw := newCompressWriter(w)
			ow = cw
			defer cw.Close()
		}

		if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") {
			cr, err := newCompressReader(r.Body)
			if err != nil {
				http.Error(w, "invalid gzip body", http.StatusBadRequest)
				return
			}
			r.Body = cr
			defer cr.Close()
		}

		next.ServeHTTP(ow, r)
	})
}
