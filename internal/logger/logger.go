// Package logger собирает общий zap-логгер и предоставляет middleware
// RequestLogger, пишущее одну строку на каждый HTTP-запрос. Логгер - явная
// зависимость: New создаёт его, а вызывающая сторона прокидывает *zap.Logger
// туда, где он нужен (нет глобального синглтона).
package logger

import (
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New создаёт production-логгер с заданным уровнем. Вызывается один раз в main,
// результат прокидывается явно - рядом со Storage и Auditor.
func New(level string) (*zap.Logger, error) {
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return nil, err
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = lvl
	// Читаемое "1.234ms" вместо секунд-как-float (0.001234) из production-конфига.
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	return cfg.Build()
}

// responseData хранит то, что нельзя прочитать из http.ResponseWriter
// постфактум: итоговый статус и суммарный размер записанного тела.
type responseData struct {
	status int
	size   int
}

// loggingResponseWriter оборачивает http.ResponseWriter, чтобы перехватывать
// Write/WriteHeader и параллельно писать в responseData, не меняя сам ответ.
type loggingResponseWriter struct {
	http.ResponseWriter
	responseData *responseData
}

func (w *loggingResponseWriter) Write(b []byte) (int, error) {
	size, err := w.ResponseWriter.Write(b)
	w.responseData.size += size
	return size, err
}

func (w *loggingResponseWriter) WriteHeader(statusCode int) {
	w.ResponseWriter.WriteHeader(statusCode)
	w.responseData.status = statusCode
}

// RequestLogger - конструктор middleware: принимает логгер и возвращает
// обёртку, пишущую один Info-объект на запрос (URI/метод/длительность запроса
// и статус/размер ответа). Логгер передаётся явно, а не берётся из глобала.
func RequestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// status по умолчанию 200: если хендлер вызывает только Write без явного
			// WriteHeader (как IndexHandler.Index), net/http сам подставляет 200 -
			// наш wrapper должен логировать то же самое, а не 0.
			data := &responseData{status: http.StatusOK}
			lw := &loggingResponseWriter{
				ResponseWriter: w,
				responseData:   data,
			}

			h.ServeHTTP(lw, r)

			log.Info("request completed",
				zap.String("uri", r.RequestURI),
				zap.String("method", r.Method),
				zap.Duration("duration", time.Since(start)),
				zap.Int("status", data.status),
				zap.Int("size", data.size),
			)
		})
	}
}
