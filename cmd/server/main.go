package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dismoralzor/metalert/internal/handler"
	"github.com/dismoralzor/metalert/internal/repository"
)

func main() {
	cfg := parseFlags()

	storage := repository.NewMemStorage()
	updateHandler := handler.NewUpdateHandler(storage)
	valueHandler := handler.NewValueHandler(storage)
	indexHandler := handler.NewIndexHandler(storage)

	r := chi.NewRouter()
	r.Post("/update/{type}/{name}/{value}", updateHandler.Update)
	r.Get("/value/{type}/{name}", valueHandler.Value)
	r.Get("/", indexHandler.Index)

	if err := http.ListenAndServe(cfg.addr, r); err != nil {
		log.Fatal(err)
	}
}
