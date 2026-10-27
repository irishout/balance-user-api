package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
	"github.com/irishout/balance-user-api/internal/transport"
)

func main() {
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal("DATABASE is not available", err)
	}
	defer pool.Close()

	err = pool.Ping(ctx)
	if err != nil {
		log.Fatal("DATABASE is not available", err)
	} else {
		log.Println("Connected")
	}

	storage := local.NewMapStorage()
	svc := service.NewService(storage)

	handler := transport.NewHandler(svc)
	router := transport.NewRouter(handler)

	log.Println("Starting server on :8080")
	err = http.ListenAndServe(":8080", router)
	if err != nil {
		log.Fatal(err)
	}
}
