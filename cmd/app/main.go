package main

import (
	"log"
	"net/http"

	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
	"github.com/irishout/balance-user-api/internal/transport"
)

func main() {

	storage := local.NewMapStorage()
	svc := service.NewService(storage)

	handler := transport.NewHandler(svc)
	router := transport.NewRouter(handler)

	log.Println("Starting server on :8080")
	err := http.ListenAndServe(":8080", router)
	if err != nil {
		log.Fatal(err)
	}
}
