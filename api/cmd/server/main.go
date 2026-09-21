package main

import (
	"log"
	"net/http"

	"github.com/smartattend/api/internal/config"
	"github.com/smartattend/api/internal/httpapi"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	handler := httpapi.NewRouter(cfg)

	log.Println("api listening on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatal(err)
	}
}
