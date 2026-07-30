package main

import (
	"log"
	"net/http"

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

	storage := repository.NewMemStorage()
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

	logger.Log.Info("starting server", zap.String("addr", cfg.addr))
	if err := http.ListenAndServe(cfg.addr, r); err != nil {
		logger.Log.Fatal(err.Error())
	}
}
