package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
	"github.com/irishout/balance-user-api/internal/storage/postgresdb"
	"github.com/irishout/balance-user-api/internal/transport"
)

func main() {
	ctx := context.Background()
	storageType := os.Getenv("STORAGE")
	var storage service.Storage
	switch storageType {
	case "postgres":
		dbURL := os.Getenv("DATABASE_URL")
		if dbURL == "" {
			log.Fatal("DATABASE_URL is not set")
		}
		pool, err := pgxpool.New(ctx, dbURL)
		if err != nil {
			log.Fatal("DATABASE is not available", err)
		}
		defer pool.Close()

		err = pool.Ping(ctx)
		if err != nil {
			log.Fatal("DATABASE is not available", err)
		} else {
			log.Println("Connected")
			storage = postgresdb.NewPostgresStorage(pool)
		}
	case "memory":
		storage = local.NewMapStorage()
	default:
		log.Fatalf("unknown STORAGE %q, want postgres or memory", storageType)

	}

	svc := service.NewService(storage)

	handler := transport.NewHandler(svc)
	router := transport.NewRouter(handler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting server on :%s", port)
	err := http.ListenAndServe(":"+port, router)
	if err != nil {
		log.Fatal(err)
	}
}
