package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
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

	// БД опциональна: пустой dsn - работаем как раньше, на памяти/файле. Но если dsn
	// задан явно, а подключиться не удалось - это не повод тихо съехать на файловый
	// режим (пользователь думает, что пишет в БД, а метрики идут в файл), поэтому
	// падаем сразу, а не продолжаем с db == nil.
	var db *sql.DB
	if cfg.dsn != "" {
		var err error
		db, err = sql.Open("pgx", cfg.dsn)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		defer db.Close()

		// sql.Open соединение не устанавливает - без Ping неработающая БД
		// обнаружилась бы только на первом реальном запросе, а не на старте.
		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			log.Fatalf("ping db: %v", err)
		}
	}

	storage, fileMode := selectStorage(cfg, db)

	updateHandler := handler.NewUpdateHandler(storage)
	valueHandler := handler.NewValueHandler(storage)
	indexHandler := handler.NewIndexHandler(storage)
	updateJSONHandler := handler.NewUpdateJSONHandler(storage)
	updatesJSONHandler := handler.NewUpdatesJSONHandler(storage)
	valueJSONHandler := handler.NewValueJSONHandler(storage)
	pingHandler := handler.NewPingHandler(db)

	r := chi.NewRouter()
	// Use до регистрации роутов: chi паникует, если middleware добавляют
	// к роутеру, в котором уже объявлены маршруты.
	// Логгер снаружи gzip: в лог попадает размер тела, реально ушедшего в сеть.
	r.Use(logger.RequestLogger)
	r.Use(handler.GzipMiddleware)
	// ПОСЛЕ Gzip: агент считает подпись от НЕсжатого тела, значит и проверять
	// её нужно уже после распаковки. При пустом cfg.key middleware прозрачен.
	r.Use(handler.HashMiddleware(cfg.key))
	r.Post("/update/{type}/{name}/{value}", updateHandler.Update)
	r.Get("/value/{type}/{name}", valueHandler.Value)
	// Спецификация описывает JSON-эндпоинты со слешем на конце, но chi считает
	// "/update" и "/update/" разными путями - регистрируем оба варианта.
	r.Post("/update", updateJSONHandler.Update)
	r.Post("/update/", updateJSONHandler.Update)
	r.Post("/updates", updatesJSONHandler.Update)
	r.Post("/updates/", updatesJSONHandler.Update)
	r.Post("/value", valueJSONHandler.Value)
	r.Post("/value/", valueJSONHandler.Value)
	r.Get("/", indexHandler.Index)
	r.Get("/ping", pingHandler.Ping)

	saveNow := func() {
		metrics := repository.Snapshot(storage)
		if err := repository.SaveToFile(cfg.fileStoragePath, metrics); err != nil {
			logger.Log.Error("save metrics", zap.Error(err))
			return
		}
		logger.Log.Info("metrics saved", zap.Int("count", len(metrics)))
	}

	// NotifyContext - это signal.Notify и отмена контекста одним вызовом.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Тикер сохранения имеет смысл только в файловом режиме - БД сама персистентна,
	// а SavingStorage (storeInterval == 0) уже пишет синхронно на каждое обновление.
	var wg sync.WaitGroup
	if fileMode && cfg.storeInterval > 0 {
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
	if fileMode {
		saveNow()
	}
}

// selectStorage выбирает реализацию Storage по приоритету: БД > файл > память.
// fileMode сообщает вызывающей стороне, нужно ли поднимать тикер периодического
// сохранения и graceful save в файл при остановке - эти механизмы не имеют смысла
// для БД (сама персистентна) и для голой памяти (сохранять некуда).
func selectStorage(cfg config, db *sql.DB) (storage repository.Storage, fileMode bool) {
	if cfg.dsn != "" {
		if err := repository.RunMigrations(db); err != nil {
			logger.Log.Fatal("run migrations", zap.Error(err))
		}
		return repository.NewDBStorage(db), false
	}

	if cfg.fileStoragePath != "" {
		storage = repository.NewMemStorage()

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

		// Оборачиваем ПОСЛЕ восстановления: иначе Restore сам вызвал бы запись
		// в файл на каждую залитую метрику.
		if cfg.storeInterval == 0 {
			storage = repository.NewSavingStorage(storage, cfg.fileStoragePath)
		}

		return storage, true
	}

	return repository.NewMemStorage(), false
}
