package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
)

func TestDepositInvalidJSON(t *testing.T) {
	request := httptest.NewRequest("POST", "/accrual", strings.NewReader("{\"user_id\":1,"))

	rec := httptest.NewRecorder()
	h := NewHandler(nil)
	h.Deposit(rec, request)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected code 400, got %v", rec.Code)
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
