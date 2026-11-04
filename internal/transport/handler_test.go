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
