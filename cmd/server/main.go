package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/dismoralzor/metalert/internal/handler"
	"github.com/dismoralzor/metalert/internal/logger"
	"github.com/dismoralzor/metalert/internal/repository"
)

func main() {
	cfg := parseFlags()

	// Через стандартный log, а не logger.Log: до успешного Initialize
	// в синглтоне лежит no-op логгер, который проглотил бы это сообщение.
	if err := logger.Initialize(cfg.logLevel); err != nil {
		log.Fatal(err)
	}

	var storage repository.Storage = repository.NewMemStorage()

	if cfg.restore {
		metrics, err := repository.LoadFromFile(cfg.fileStoragePath)
		if err != nil {
			// Битый файл не должен мешать старту - поднимаемся с пустым хранилищем.
			logger.Log.Error("restore metrics", zap.Error(err))
		} else {
			repository.Restore(storage, metrics)
			logger.Log.Info("metrics restored", zap.Int("count", len(metrics)))
		}
	}

	// Оборачиваем ПОСЛЕ восстановления: иначе Restore сам вызвал бы запись в файл
	// на каждую залитую метрику.
	if cfg.storeInterval == 0 {
		storage = repository.NewSavingStorage(storage, cfg.fileStoragePath)
	}

	updateHandler := handler.NewUpdateHandler(storage)
	valueHandler := handler.NewValueHandler(storage)
	indexHandler := handler.NewIndexHandler(storage)
	updateJSONHandler := handler.NewUpdateJSONHandler(storage)
	valueJSONHandler := handler.NewValueJSONHandler(storage)

	r := chi.NewRouter()
	// Use до регистрации роутов: chi паникует, если middleware добавляют
	// к роутеру, в котором уже объявлены маршруты.
	// Логгер снаружи gzip: в лог попадает размер тела, реально ушедшего в сеть.
	r.Use(logger.RequestLogger)
	r.Use(handler.GzipMiddleware)
	r.Post("/update/{type}/{name}/{value}", updateHandler.Update)
	r.Get("/value/{type}/{name}", valueHandler.Value)
	// Спецификация описывает JSON-эндпоинты со слешем на конце, но chi считает
	// "/update" и "/update/" разными путями - регистрируем оба варианта.
	r.Post("/update", updateJSONHandler.Update)
	r.Post("/update/", updateJSONHandler.Update)
	r.Post("/value", valueJSONHandler.Value)
	r.Post("/value/", valueJSONHandler.Value)
	r.Get("/", indexHandler.Index)

	saveNow := func() {
		metrics, err := repository.Snapshot(storage)
		if err != nil {
			logger.Log.Error("snapshot metrics", zap.Error(err))
			return
		}
		if err := repository.SaveToFile(cfg.fileStoragePath, metrics); err != nil {
			logger.Log.Error("save metrics", zap.Error(err))
			return
		}
		logger.Log.Info("metrics saved", zap.Int("count", len(metrics)))
	}

	// NotifyContext - это signal.Notify и отмена контекста одним вызовом.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	if cfg.storeInterval > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()

			ticker := time.NewTicker(cfg.storeInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					saveNow()
				}
			}
		}()
	}

	srv := &http.Server{Addr: cfg.addr, Handler: r}

	go func() {
		<-ctx.Done()
		// Даём активным запросам дописать ответы, но не ждём вечно.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Log.Error("shutdown", zap.Error(err))
		}
	}()

	logger.Log.Info("starting server", zap.String("addr", cfg.addr))
	// После Shutdown ListenAndServe возвращает ErrServerClosed - это штатный выход.
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Log.Fatal(err.Error())
	}

	// Сначала дожидаемся остановки тикера, чтобы он не писал файл параллельно
	// с финальным сохранением.
	wg.Wait()
	saveNow()
}
