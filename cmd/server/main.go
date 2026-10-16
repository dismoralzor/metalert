// Command server - сервер сбора метрик: принимает метрики по HTTP (текстовый и
// JSON-протоколы), хранит их в памяти/файле/PostgreSQL и отдаёт по запросу.
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

	"github.com/dismoralzor/metalert/internal/audit"
	"github.com/dismoralzor/metalert/internal/handler"
	"github.com/dismoralzor/metalert/internal/logger"
	"github.com/dismoralzor/metalert/internal/repository"
	"github.com/dismoralzor/metalert/internal/retry"
)

func main() {
	cfg := parseFlags()

	// Логгер - явная зависимость: создаём здесь и прокидываем дальше. До этого
	// момента об ошибках сообщаем стандартным log (zap ещё нет).
	zlog, err := logger.New(cfg.logLevel)
	if err != nil {
		log.Fatal(err)
	}
	defer zlog.Sync()

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

	// Старт приложения - осмысленного родительского контекста ещё нет
	// (сигналы перехватим чуть ниже), поэтому Background - как и раньше
	// для Ping выше.
	storage, fileMode := selectStorage(context.Background(), cfg, db, zlog)

	// Аудитор (паттерн Наблюдатель): издатель + приёмники по конфигу.
	// Подписчиков может не быть вовсе - тогда NotifyAll просто ничего не делает.
	auditor := audit.NewSubject(zlog)
	if cfg.auditFile != "" {
		// Файл открывается в конструкторе: ошибка доступа должна валить старт
		// сразу, а не всплывать на первом же событии аудита.
		fileObs, err := audit.NewFileObserver(cfg.auditFile)
		if err != nil {
			log.Fatalf("open audit file: %v", err)
		}
		auditor.Subscribe(fileObs)
		zlog.Info("audit file observer enabled", zap.String("path", cfg.auditFile))
	}
	if cfg.auditURL != "" {
		auditor.Subscribe(audit.NewHTTPObserver(cfg.auditURL))
		zlog.Info("audit http observer enabled", zap.String("url", cfg.auditURL))
	}
	// Закрываем приёмники (например, файл) при остановке.
	defer func() {
		if err := auditor.Close(); err != nil {
			zlog.Error("close auditor", zap.Error(err))
		}
	}()

	updateHandler := handler.NewUpdateHandler(storage, auditor)
	valueHandler := handler.NewValueHandler(storage)
	indexHandler := handler.NewIndexHandler(storage)
	updateJSONHandler := handler.NewUpdateJSONHandler(storage, auditor)
	updatesJSONHandler := handler.NewUpdatesJSONHandler(storage, auditor)
	valueJSONHandler := handler.NewValueJSONHandler(storage)
	pingHandler := handler.NewPingHandler(db)

	r := chi.NewRouter()
	// Use до регистрации роутов: chi паникует, если middleware добавляют
	// к роутеру, в котором уже объявлены маршруты.
	// Логгер снаружи gzip: в лог попадает размер тела, реально ушедшего в сеть.
	r.Use(logger.RequestLogger(zlog))
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

	// saveNow принимает ctx снаружи - и периодический тикер, и финальный
	// graceful save заводят под него СВОЙ контекст с таймаутом (см. вызовы ниже):
	// это фоновая операция вне цепочки запросов, у неё нет естественного
	// "родительского" контекста вроде r.Context().
	saveNow := func(ctx context.Context) {
		metrics := repository.Snapshot(ctx, storage)
		if err := repository.SaveToFile(cfg.fileStoragePath, metrics); err != nil {
			zlog.Error("save metrics", zap.Error(err))
			return
		}
		zlog.Info("metrics saved", zap.Int("count", len(metrics)))
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
					saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					saveNow(saveCtx)
					cancel()
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
			zlog.Error("shutdown", zap.Error(err))
		}
	}()

	zlog.Info("starting server", zap.String("addr", cfg.addr))
	// После Shutdown ListenAndServe возвращает ErrServerClosed - это штатный выход.
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		// Стандартный log.Fatal, а не zlog.Fatal: явный os.Exit(1) вместо
		// того, чтобы полагаться на onFatal-хук zap-логгера. Это происходит ДО
		// получения сигнала (например, "address already in use" при старте) -
		// ctx ещё не отменён, а значит wg.Wait() ниже заблокировалась бы навсегда:
		// тикер сохранения ждёт именно ctx.Done(), которого в этом сценарии не будет.
		// Живой процесс без слушающего порта недопустим - выходим прямо здесь.
		log.Fatalf("listen and serve: %v", err)
	}

	// Сначала дожидаемся остановки тикера, чтобы он не писал файл параллельно
	// с финальным сохранением. Оборачиваем в select с таймаутом: если фоновая
	// горутина по какой-то причине не отреагирует на ctx.Done() (баг, зависание),
	// процесс всё равно должен завершиться, а не держать порт занятым бесконечно.
	waitDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(15 * time.Second):
		zlog.Error("background goroutines did not stop in time, exiting anyway")
	}

	if fileMode {
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		saveNow(saveCtx)
	}
}

// selectStorage выбирает реализацию Storage по приоритету: БД > файл > память.
// fileMode сообщает вызывающей стороне, нужно ли поднимать тикер периодического
// сохранения и graceful save в файл при остановке - эти механизмы не имеют смысла
// для БД (сама персистентна) и для голой памяти (сохранять некуда).
func selectStorage(ctx context.Context, cfg config, db *sql.DB, zlog *zap.Logger) (storage repository.Storage, fileMode bool) {
	if cfg.dsn != "" {
		if err := repository.RunMigrations(db); err != nil {
			zlog.Fatal("run migrations", zap.Error(err))
		}
		return repository.NewDBStorage(db, retry.DefaultPolicy(), zlog), false
	}

	if cfg.fileStoragePath != "" {
		storage = repository.NewMemStorage()

		if cfg.restore {
			metrics, err := repository.LoadFromFile(cfg.fileStoragePath)
			if err != nil {
				// Битый файл не должен мешать старту - поднимаемся с пустым хранилищем.
				zlog.Error("restore metrics", zap.Error(err))
			} else {
				repository.Restore(ctx, storage, metrics)
				zlog.Info("metrics restored", zap.Int("count", len(metrics)))
			}
		}

		// Оборачиваем ПОСЛЕ восстановления: иначе Restore сам вызвал бы запись
		// в файл на каждую залитую метрику.
		if cfg.storeInterval == 0 {
			storage = repository.NewSavingStorage(storage, cfg.fileStoragePath, zlog)
		}

		return storage, true
	}

	return repository.NewMemStorage(), false
}
