package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/irishout/balance-user-api/internal/domain"
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
	_, err := svc.Deposit(context.Background(), 1, 500, "")
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
	_, err := svc.Deposit(context.Background(), 1, 500, "")
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
	_, err := svc.Deposit(context.Background(), 1, 1000, "")
	if err != nil {
		t.Fatalf("got Deposit error %v", err)
	}
	_, err = svc.Deposit(context.Background(), 2, 500, "")
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

	if _, err := svc.Deposit(context.Background(), 1, 1000, ""); err != nil {
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

type operationsStorageStub struct {
	service.Storage
	listOperations func(userID int64, limit, offset int, sortBy string) ([]domain.Operation, error)
}

func (s *operationsStorageStub) ListOperations(_ context.Context, userID int64, limit, offset int, sortBy string) ([]domain.Operation, error) {
	return s.listOperations(userID, limit, offset, sortBy)
}

func TestOperationsSuccess(t *testing.T) {
	recipientID := int64(2)
	tests := []struct {
		name       string
		path       string
		limit      int
		offset     int
		sortBy     string
		operations []domain.Operation
		wantJSON   string
	}{
		{
			name:   "default pagination and sort",
			path:   "/operations/2",
			limit:  20,
			sortBy: "date",
			operations: []domain.Operation{{
				ID: 3, Type: "transfer", Date: time.Date(2026, time.November, 26, 15, 0, 0, 0, time.UTC),
				UserID: 1, RecipientID: &recipientID, Amount: 300, Comment: "Возврат долга",
			}},
			wantJSON: `{"operations":[{
				"id":3,"operation_type":"transfer","operation_date":"2026-11-26T15:00:00Z",
				"user_id":1,"recipient_id":2,"amount":300,"comment":"Возврат долга"
			}]}`,
		},
		{
			name:     "empty page with amount sort",
			path:     "/operations/2?page=3&page_size=2&sort=amount",
			limit:    2,
			offset:   4,
			sortBy:   "amount",
			wantJSON: `{"operations":[]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			storage := &operationsStorageStub{
				listOperations: func(userID int64, limit, offset int, sortBy string) ([]domain.Operation, error) {
					calls++
					if userID != 2 || limit != test.limit || offset != test.offset || sortBy != test.sortBy {
						t.Errorf("storage got userID=%d, limit=%d, offset=%d, sortBy=%q; want 2, %d, %d, %q",
							userID, limit, offset, sortBy, test.limit, test.offset, test.sortBy)
					}
					return test.operations, nil
				},
			}
			router := NewRouter(NewHandler(service.NewService(storage)))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))

			if response.Code != http.StatusOK || calls != 1 {
				t.Fatalf("status=%d, storage calls=%d; want 200, 1", response.Code, calls)
			}
			var got, want any
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode operations response: %v", err)
			}
			if err := json.Unmarshal([]byte(test.wantJSON), &want); err != nil {
				t.Fatalf("decode expected response: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("response = %s, want %s", response.Body.String(), test.wantJSON)
			}
		})
	}
}

func TestOperationsInvalidParameters(t *testing.T) {
	router := NewRouter(NewHandler(service.NewService(nil)))
	tests := []struct {
		path    string
		message string
	}{
		{"/operations/abc", "user_id must be a number"},
		{"/operations/0", "user_id must be greater than 0"},
		{"/operations/2?page=abc", "page must be a number"},
		{"/operations/2?page=0", "page must be positive"},
		{"/operations/2?page_size=abc", "page_size must be a number"},
		{"/operations/2?page_size=0", "page_size must be positive"},
		{"/operations/2?sort=unknown", "sort must be date or amount"},
		{"/operations/2?sort=%22date%22", "sort must be date or amount"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; response = %s", response.Code, response.Body.String())
			}
			var body responseError
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error response: %v", err)
			}
			if body.Err != test.message {
				t.Errorf("error = %q, want %q", body.Err, test.message)
			}
		})
	}
}

func TestOperationsMemoryStorageUnsupported(t *testing.T) {
	router := NewRouter(NewHandler(service.NewService(local.NewMapStorage())))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/operations/2", nil))

	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; response = %s", response.Code, response.Body.String())
	}
	var body responseError
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Err != "operation history requires PostgreSQL" {
		t.Errorf("unexpected error message %q", body.Err)
	}
}
