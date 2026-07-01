package main

import (
	"log"
	"net/http"

	"github.com/dismoralzor/metalert/internal/handler"
	"github.com/dismoralzor/metalert/internal/repository"
)

func main() {
	storage := repository.NewMemStorage()
	updateHandler := handler.NewUpdateHandler(storage)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /update/{type}/{name}/{value}", updateHandler.Update)

	if err := http.ListenAndServe("localhost:8080", mux); err != nil {
		log.Fatal(err)
	}
}
