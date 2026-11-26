package transport

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/irishout/balance-user-api/internal/service"
)

type requestOneID struct {
	UserID int64 `json:"user_id"`
	Amount int64 `json:"amount"`
}

type responseWithBalance struct {
	Balance int64 `json:"balance"`
}

type responseError struct {
	Err string `json:"error"`
}

type requestTransfer struct {
	UserFromID int64 `json:"user_from_id"`
	UserToID   int64 `json:"user_to_id"`
	Amount     int64 `json:"amount"`
}

type responseTransfer struct {
	BalanceUserFrom int64 `json:"balance_user_from"`
	BalanceUserTo   int64 `json:"balance_user_to"`
}

type requestOperations struct {
	UserID int64
}

type responseOperations struct {
	Operations []domain.Operation `json:"operations"`
}

type Handler struct {
	svc *service.Service
}

func NewHandler(s *service.Service) *Handler {
	return &Handler{
		svc: s,
	}
}

func writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Println("Can't write JSON response:", err)
	}

}

func writeError(w http.ResponseWriter, code int, message string) {
	response := responseError{
		Err: message,
	}
	writeJSON(w, code, response)
}

func errorHandler(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, domain.ErrBalanceOverflow):
		writeError(w, http.StatusConflict, "balance overflow")
	case errors.Is(err, domain.ErrInvalidAmount):
		writeError(w, http.StatusBadRequest, "amount must be greater than 0")
	case errors.Is(err, domain.ErrInvalidUserID):
		writeError(w, http.StatusBadRequest, "user_id must be greater than 0")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "user_id not found")
	case errors.Is(err, domain.ErrInvalidSenderID):
		writeError(w, http.StatusBadRequest, "user_from_id must be greater than 0")
	case errors.Is(err, domain.ErrInvalidRecipientID):
		writeError(w, http.StatusBadRequest, "user_to_id must be greater than 0")
	case errors.Is(err, domain.ErrSelfTransfer):
		writeError(w, http.StatusBadRequest, "user_to_id and user_from_id must be different")
	case errors.Is(err, domain.ErrSenderNotFound):
		writeError(w, http.StatusNotFound, "user_from_id not found")
	case errors.Is(err, domain.ErrRecipientNotFound):
		writeError(w, http.StatusNotFound, "user_to_id not found")
	case errors.Is(err, domain.ErrInsufficientFunds):
		writeError(w, http.StatusConflict, "insufficient funds")
	case errors.Is(err, domain.ErrInvalidOperationSort):
		writeError(w, http.StatusBadRequest, "sort must be date or amount")
	case errors.Is(err, errors.ErrUnsupported):
		writeError(w, http.StatusNotImplemented, "operation history requires PostgreSQL")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (h *Handler) Deposit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req requestOneID
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	balance, err := h.svc.Deposit(ctx, req.UserID, req.Amount)
	if err != nil {
		errorHandler(w, err)
		return
	}
	writeJSON(w, http.StatusOK, responseWithBalance{Balance: balance})
}
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req requestOneID
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	balance, err := h.svc.Withdraw(ctx, req.UserID, req.Amount)
	if err != nil {
		errorHandler(w, err)
		return
	}
	writeJSON(w, http.StatusOK, responseWithBalance{Balance: balance})
}

func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req requestTransfer
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	balanceFrom, balanceTo, err := h.svc.Transfer(ctx, req.UserFromID, req.UserToID, req.Amount)
	if err != nil {
		errorHandler(w, err)
		return
	}
	writeJSON(w, http.StatusOK, responseTransfer{BalanceUserFrom: balanceFrom, BalanceUserTo: balanceTo})
}

func (h *Handler) Balance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userIDStr := r.PathValue("user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_id must be a number")
		return
	}

	balance, err := h.svc.Balance(ctx, userID)
	if err != nil {
		errorHandler(w, err)
		return
	}
	writeJSON(w, http.StatusOK, responseWithBalance{Balance: balance})
}

func (h *Handler) Operations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userIDStr := r.PathValue("user_id")
	query := r.URL.Query()
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "user_id must be a number")
		return
	}

	req := requestOperations{UserID: userID}
	var page, pageSize int
	pageStr := query.Get("page")
	if pageStr == "" {
		page = 1
	} else {
		page, err = strconv.Atoi(pageStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "page must be a number")
			return
		}
	}
	if page <= 0 {
		writeError(w, http.StatusBadRequest, "page must be positive")
		return
	}

	pageSizeStr := query.Get("page_size")
	if pageSizeStr == "" {
		pageSize = 20
	} else {
		pageSize, err = strconv.Atoi(pageSizeStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "page_size must be a number")
			return
		}
	}
	if pageSize <= 0 {
		writeError(w, http.StatusBadRequest, "page_size must be positive")
		return
	}

	sortType := query.Get("sort")

	operations, err := h.svc.ListOperations(ctx, req.UserID, pageSize, (page-1)*pageSize, sortType)
	if err != nil {
		errorHandler(w, err)
		return
	}
	response := responseOperations{Operations: make([]domain.Operation, 0, len(operations))}
	for _, operation := range operations {
		response.Operations = append(response.Operations, domain.Operation{
			ID:          operation.ID,
			Type:        operation.Type,
			Date:        operation.Date,
			UserID:      operation.UserID,
			RecipientID: operation.RecipientID,
			Amount:      operation.Amount,
			Comment:     operation.Comment,
		})
	}
	writeJSON(w, http.StatusOK, response)
}
