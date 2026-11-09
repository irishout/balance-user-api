package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	balancev1 "github.com/irishout/balance-user-api/gen/go/balance/v1"

	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
	"github.com/irishout/balance-user-api/internal/storage/postgresdb"
	"github.com/irishout/balance-user-api/internal/transport"
	"github.com/irishout/balance-user-api/internal/transport/grpcapi"
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting server on :%s", port)

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) //graceful shutdown на остановку Ctrl+C или docker compose stop
	defer stop()

	transportType := os.Getenv("TRANSPORT")
	switch transportType {
	case "http":
		handler := transport.NewHandler(svc)
		router := transport.NewRouter(handler)
		server := http.Server{
			Addr:    ":" + port,
			Handler: transport.LoggingMiddleware(router),
		}

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

	case "grpc":
		handler := grpcapi.NewHandler(svc)
		server := grpc.NewServer()
		balancev1.RegisterBalanceServiceServer(server, handler)
		listener, err := net.Listen("tcp", ":"+port)
		if err != nil {
			log.Fatal("Cannot open TCP connection", err)
		}
		go func() {
			err := server.Serve(listener)
			if err != nil && err != grpc.ErrServerStopped {
				log.Fatal(err)
			}
		}()
		<-stopCtx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		stopped := make(chan struct{})

		go func() {
			server.GracefulStop()
			close(stopped)
		}()

		select {
		case <-stopped:
			log.Println("Successful server stop")
		case <-shutdownCtx.Done():
			log.Println("Server stop timeout")
			server.Stop()
		}

	default:
		log.Fatalf("unknown TRANSPORT %q, want http or grpc", transportType)
	}

}
