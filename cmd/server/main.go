package main

import (
	"log"
	"net/http"

	"github.com/mgfan1/go-metrics/internal/handler"
	"github.com/mgfan1/go-metrics/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	store := storage.NewMemStorage()
	h := handler.New(store)

	return http.ListenAndServe(":8080", h.Router())
}
