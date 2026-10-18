package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
)

func main() {
	http.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "pong")
	})

	storage := local.NewMapStorage()
	srv := service.NewService(storage)
	//test tmp lines
	srv.Deposit(1, 3000)
	srv.Deposit(2, 4000)
	fmt.Println(storage.Balance(1))
	fmt.Println(storage.Balance(2))
	//end of tmp lines

	log.Println("Starting server on :8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
