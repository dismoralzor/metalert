package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/dismoralzor/metalert/internal/handler"
	"github.com/dismoralzor/metalert/internal/repository"
)

func main() {
	storage := repository.NewMemStorage()
	updateHandler := handler.NewUpdateHandler(storage)
	valueHandler := handler.NewValueHandler(storage)
	indexHandler := handler.NewIndexHandler(storage)

	r := chi.NewRouter()
	r.Post("/update/{type}/{name}/{value}", updateHandler.Update)
	r.Get("/value/{type}/{name}", valueHandler.Value)
	r.Get("/", indexHandler.Index)

	if err := http.ListenAndServe("localhost:8080", r); err != nil {
		log.Fatal(err)
	}
}
