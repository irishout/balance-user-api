package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	server := http.Server{
		Addr:    ":" + port,
		Handler: transport.LoggingMiddleware(router),
	}

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) //graceful shutdown на остановку Ctrl+C или docker compose stop
	defer stop()

	go func() {
		err := server.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	<-stopCtx.Done() //чтение из канала, ждем сигнал после которого канал закроется

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := server.Shutdown(shutdownCtx)
	if err != nil {
		log.Println("Cannot shutdown server", err)
		server.Close()
	}

}
