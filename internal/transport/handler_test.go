package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
)

func TestDepositInvalidJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/accrual", strings.NewReader("{\"user_id\":1,"))

	rec := httptest.NewRecorder()
	h := NewHandler(nil)
	h.Deposit(rec, request)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected code %d, got %v", http.StatusBadRequest, rec.Code)
	}

	decoder := json.NewDecoder(rec.Body)
	var errStruct responseError
	err := decoder.Decode(&errStruct)
	if err != nil {
		t.Fatalf("got decode error %v", err)
	}
	if errStruct.Err != "invalid JSON body" {
		t.Fatalf("expected error \"invalid JSON body\", got %v", errStruct.Err)
	}
}

func TestDepositSuccess(t *testing.T) {
	storage := local.NewMapStorage()
	svc := service.NewService(storage)
	h := NewHandler(svc)

	request := httptest.NewRequest(http.MethodPost, "/accrual", strings.NewReader(`{"user_id":1,"amount":1000}`))
	rec := httptest.NewRecorder()
	h.Deposit(rec, request)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode deposit response: %v", err)
	}
	if response.Balance != 1000 {
		t.Errorf("expected balance 1000, got %d", response.Balance)
	}
}

func TestWithdrawInsufficientFunds(t *testing.T) {
	storage := local.NewMapStorage()
	svc := service.NewService(storage)
	h := NewHandler(svc)
	_, err := svc.Deposit(context.Background(), 1, 500)
	if err != nil {
		t.Fatalf("got Deposit error %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/debiting", strings.NewReader(`{"user_id":1,"amount":700}`))
	rec := httptest.NewRecorder()
	h.Withdraw(rec, request)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}

	decoder := json.NewDecoder(rec.Body)
	var errStruct responseError
	err = decoder.Decode(&errStruct)
	if err != nil {
		t.Fatalf("got decode error %v", err)
	}
	if errStruct.Err != "insufficient funds" {
		t.Fatalf("expected error \"insufficient funds\", got %v", errStruct.Err)
	}
}

func TestWithdrawSuccess(t *testing.T) {
	storage := local.NewMapStorage()
	svc := service.NewService(storage)
	h := NewHandler(svc)
	_, err := svc.Deposit(context.Background(), 1, 500)
	if err != nil {
		t.Fatalf("got Deposit error %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/debiting", strings.NewReader(`{"user_id":1,"amount":300}`))
	rec := httptest.NewRecorder()
	h.Withdraw(rec, request)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode withdraw response: %v", err)
	}
	if response.Balance != 200 {
		t.Errorf("expected balance 200, got %d", response.Balance)
	}
}
func TestTransferSuccess(t *testing.T) {
	storage := local.NewMapStorage()
	svc := service.NewService(storage)
	h := NewHandler(svc)
	_, err := svc.Deposit(context.Background(), 1, 1000)
	if err != nil {
		t.Fatalf("got Deposit error %v", err)
	}
	_, err = svc.Deposit(context.Background(), 2, 500)
	if err != nil {
		t.Fatalf("got Deposit error %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/transfer", strings.NewReader(`{"user_from_id":1,"user_to_id":2,"amount":300}`))
	rec := httptest.NewRecorder()
	h.Transfer(rec, request)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response struct {
		BalanceUserFrom int64 `json:"balance_user_from"`
		BalanceUserTo   int64 `json:"balance_user_to"`
	}

	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode transfer response: %v", err)
	}
	if response.BalanceUserFrom != 700 {
		t.Errorf("expected balance 700, got %d", response.BalanceUserFrom)
	}
	if response.BalanceUserTo != 800 {
		t.Errorf("expected balance 800, got %d", response.BalanceUserTo)
	}

}
func TestBalanceSuccess(t *testing.T) {
	storage := local.NewMapStorage()
	svc := service.NewService(storage)
	h := NewHandler(svc)
	router := NewRouter(h)

	if _, err := svc.Deposit(context.Background(), 1, 1000); err != nil {
		t.Fatalf("prepare balance: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/balance/1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, request)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response struct {
		Balance int64 `json:"balance"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode balance response: %v", err)
	}
	if response.Balance != 1000 {
		t.Errorf("expected balance 1000, got %d", response.Balance)
	}
}

func TestBalanceNotFound(t *testing.T) {
	storage := local.NewMapStorage()
	svc := service.NewService(storage)
	h := NewHandler(svc)
	router := NewRouter(h)

	request := httptest.NewRequest(http.MethodGet, "/balance/1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, request)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}

	var response responseError
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode balance error response: %v", err)
	}
	if response.Err != "user_id not found" {
		t.Errorf("expected error %q, got %q", "user_id not found", response.Err)
	}
}

func TestTransferSelfTransfer(t *testing.T) {
	storage := local.NewMapStorage()
	svc := service.NewService(storage)
	h := NewHandler(svc)

	request := httptest.NewRequest(http.MethodPost, "/transfer", strings.NewReader(`{"user_from_id":1,"user_to_id":1,"amount":300}`))
	rec := httptest.NewRecorder()
	h.Transfer(rec, request)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	decoder := json.NewDecoder(rec.Body)
	var errStruct responseError
	err := decoder.Decode(&errStruct)
	if err != nil {
		t.Fatalf("got decode error %v", err)
	}
	if errStruct.Err != "user_to_id and user_from_id must be different" {
		t.Fatalf("expected error \"user_to_id and user_from_id must be different\", got %v", errStruct.Err)
	}
}
