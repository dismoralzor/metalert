package logger

import (
	"net/http"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Log - синглтон логгера. По умолчанию NewNop(): пакеты, которые импортируют
// logger, но забыли/не успели вызвать Initialize (например, в тестах),
// получают безопасный no-op вместо паники на nil-логгере.
var Log *zap.Logger = zap.NewNop()

// Initialize заменяет Log на настоящий логгер с заданным уровнем.
func Initialize(level string) error {
	lvl, err := zap.ParseAtomicLevel(level)
	if err != nil {
		return err
	}

	cfg := zap.NewProductionConfig()
	cfg.Level = lvl
	// Читаемое "1.234ms" вместо секунд-как-float (0.001234) из production-конфига.
	cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder

	zl, err := cfg.Build()
	if err != nil {
		return err
	}

	Log = zl
	return nil
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

// RequestLogger - middleware, логирующее один Info-объект на запрос:
// URI/метод/длительность запроса и статус/размер ответа.
func RequestLogger(h http.Handler) http.Handler {
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

		Log.Info("request completed",
			zap.String("uri", r.RequestURI),
			zap.String("method", r.Method),
			zap.Duration("duration", time.Since(start)),
			zap.Int("status", data.status),
			zap.Int("size", data.size),
		)
	})
}
