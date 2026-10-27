package transport

import (
	"fmt"
	"net/http"
)

func NewRouter(h *Handler) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "pong")
	})
	mux.HandleFunc("POST /accrual", h.Deposit)
	mux.HandleFunc("POST /debiting", h.Withdraw)
	mux.HandleFunc("POST /transfer", h.Transfer)
	mux.HandleFunc("GET /balance/{user_id}", h.Balance)

	return mux
}
