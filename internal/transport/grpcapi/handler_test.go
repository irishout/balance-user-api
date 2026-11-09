package grpcapi

import (
	"context"
	"errors"
	"math"
	"testing"

	balancev1 "github.com/irishout/balance-user-api/gen/go/balance/v1"
	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/irishout/balance-user-api/internal/service"
	"github.com/irishout/balance-user-api/internal/storage/local"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestToGRPCError(t *testing.T) {
	tests := []struct {
		name        string
		inputErr    error
		wantCode    codes.Code
		wantMessage string
	}{
		{
			name:        "nil error",
			inputErr:    nil,
			wantCode:    codes.OK,
			wantMessage: "",
		},
		{
			name:        "invalid user ID",
			inputErr:    domain.ErrInvalidUserID,
			wantCode:    codes.InvalidArgument,
			wantMessage: "user_id must be greater than 0",
		},
		{
			name:        "user not found",
			inputErr:    domain.ErrNotFound,
			wantCode:    codes.NotFound,
			wantMessage: "user_id not found",
		},
		{
			name:        "unknown error",
			inputErr:    errors.New("storage unavailable"),
			wantCode:    codes.Internal,
			wantMessage: "internal error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotErr := toGRPCError(tc.inputErr)
			if tc.inputErr == nil && gotErr != nil {
				t.Fatalf("expected nil error, got %v", gotErr)
			}
			if gotCode := status.Code(gotErr); gotCode != tc.wantCode {
				t.Errorf("expected code %v, got %v", tc.wantCode, gotCode)
			}
			if gotMessage := status.Convert(gotErr).Message(); gotMessage != tc.wantMessage {
				t.Errorf("expected message %q, got %q", tc.wantMessage, gotMessage)
			}
		})
	}
}

func TestGetBalanceSuccess(t *testing.T) {
	for _, wantBalance := range []int64{1000, 0} {
		name := "positive balance"
		if wantBalance == 0 {
			name = "zero balance"
		}
		t.Run(name, func(t *testing.T) {
			h, svc := newHandlerWithBalances(t, map[int64]int64{7: 1000})
			ctx := context.Background()
			if wantBalance == 0 {
				if _, err := svc.Withdraw(ctx, 7, 1000); err != nil {
					t.Fatalf("prepare zero balance: %v", err)
				}
			}

			response, err := h.GetBalance(ctx, &balancev1.BalanceRequest{UserId: 7})
			if err != nil {
				t.Fatalf("GetBalance: %v", err)
			}
			if response == nil {
				t.Fatal("expected a balance response, got nil")
			}
			if got := response.GetBalance(); got != wantBalance {
				t.Errorf("expected balance %d, got %d", wantBalance, got)
			}
		})
	}
}

func TestGetBalanceNotFound(t *testing.T) {
	h, _ := newHandlerWithBalances(t, nil)

	response, err := h.GetBalance(context.Background(), &balancev1.BalanceRequest{UserId: 7})
	if response != nil {
		t.Errorf("expected nil response, got %v", response)
	}
	requireGRPCError(t, err, codes.NotFound, "user_id not found")
}

func TestDepositSuccess(t *testing.T) {
	tests := []struct {
		name           string
		initialBalance int64
		amount         int64
		wantBalance    int64
	}{
		{name: "new user", amount: 1000, wantBalance: 1000},
		{name: "existing user", initialBalance: 500, amount: 300, wantBalance: 800},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			initial := make(map[int64]int64)
			if tc.initialBalance > 0 {
				initial[7] = tc.initialBalance
			}
			h, svc := newHandlerWithBalances(t, initial)

			response, err := h.Deposit(context.Background(), &balancev1.DepositRequest{UserId: 7, Amount: tc.amount})
			if err != nil {
				t.Fatalf("Deposit: %v", err)
			}
			if response == nil {
				t.Fatal("expected a balance response, got nil")
			}
			if got := response.GetBalance(); got != tc.wantBalance {
				t.Errorf("expected balance %d, got %d", tc.wantBalance, got)
			}
			requireStoredBalance(t, svc, 7, tc.wantBalance)
		})
	}
}

func TestDepositInvalidAmount(t *testing.T) {
	for _, tc := range []struct {
		name   string
		amount int64
	}{
		{name: "zero amount", amount: 0},
		{name: "negative amount", amount: -100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, svc := newHandlerWithBalances(t, nil)
			response, err := h.Deposit(context.Background(), &balancev1.DepositRequest{UserId: 7, Amount: tc.amount})
			if response != nil {
				t.Errorf("expected nil response, got %v", response)
			}
			requireGRPCError(t, err, codes.InvalidArgument, "amount must be greater than 0")
			if _, err := svc.Balance(context.Background(), 7); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("expected user to remain absent, got error %v", err)
			}
		})
	}
}

func TestWithdrawSuccess(t *testing.T) {
	tests := []struct {
		name        string
		amount      int64
		wantBalance int64
	}{
		{name: "partial withdrawal", amount: 300, wantBalance: 700},
		{name: "entire balance", amount: 1000, wantBalance: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, svc := newHandlerWithBalances(t, map[int64]int64{7: 1000})
			response, err := h.Withdraw(context.Background(), &balancev1.WithdrawRequest{UserId: 7, Amount: tc.amount})
			if err != nil {
				t.Fatalf("Withdraw: %v", err)
			}
			if response == nil {
				t.Fatal("expected a balance response, got nil")
			}
			if got := response.GetBalance(); got != tc.wantBalance {
				t.Errorf("expected balance %d, got %d", tc.wantBalance, got)
			}
			requireStoredBalance(t, svc, 7, tc.wantBalance)
		})
	}
}

func TestWithdrawInsufficientFunds(t *testing.T) {
	h, svc := newHandlerWithBalances(t, map[int64]int64{7: 1000})
	response, err := h.Withdraw(context.Background(), &balancev1.WithdrawRequest{UserId: 7, Amount: 1001})
	if response != nil {
		t.Errorf("expected nil response, got %v", response)
	}
	requireGRPCError(t, err, codes.FailedPrecondition, "insufficient funds")
	requireStoredBalance(t, svc, 7, 1000)
}

func TestTransferSuccess(t *testing.T) {
	h, svc := newHandlerWithBalances(t, map[int64]int64{7: 1000, 9: 500})
	response, err := h.Transfer(context.Background(), &balancev1.TransferRequest{
		UserFromId: 7,
		UserToId:   9,
		Amount:     300,
	})
	if err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if response == nil {
		t.Fatal("expected a transfer response, got nil")
	}
	if got := response.GetBalanceUserFrom(); got != 700 {
		t.Errorf("expected sender balance 700, got %d", got)
	}
	if got := response.GetBalanceUserTo(); got != 800 {
		t.Errorf("expected recipient balance 800, got %d", got)
	}
	requireStoredBalance(t, svc, 7, 700)
	requireStoredBalance(t, svc, 9, 800)
}

func TestTransferErrors(t *testing.T) {
	tests := []struct {
		name             string
		fromID           int64
		toID             int64
		amount           int64
		recipientBalance int64
		wantCode         codes.Code
		wantMessage      string
	}{
		{
			name: "self transfer", fromID: 7, toID: 7, amount: 100, recipientBalance: 500,
			wantCode: codes.InvalidArgument, wantMessage: "user_to_id and user_from_id must be different",
		},
		{
			name: "sender not found", fromID: 99, toID: 9, amount: 100, recipientBalance: 500,
			wantCode: codes.NotFound, wantMessage: "user_from_id not found",
		},
		{
			name: "recipient not found", fromID: 7, toID: 99, amount: 100, recipientBalance: 500,
			wantCode: codes.NotFound, wantMessage: "user_to_id not found",
		},
		{
			name: "insufficient funds", fromID: 7, toID: 9, amount: 1001, recipientBalance: 500,
			wantCode: codes.FailedPrecondition, wantMessage: "insufficient funds",
		},
		{
			name: "recipient overflow", fromID: 7, toID: 9, amount: 1, recipientBalance: math.MaxInt64,
			wantCode: codes.OutOfRange, wantMessage: "balance overflow",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, svc := newHandlerWithBalances(t, map[int64]int64{7: 1000, 9: tc.recipientBalance})
			response, err := h.Transfer(context.Background(), &balancev1.TransferRequest{
				UserFromId: tc.fromID,
				UserToId:   tc.toID,
				Amount:     tc.amount,
			})
			if response != nil {
				t.Errorf("expected nil response, got %v", response)
			}
			requireGRPCError(t, err, tc.wantCode, tc.wantMessage)
			requireStoredBalance(t, svc, 7, 1000)
			requireStoredBalance(t, svc, 9, tc.recipientBalance)
		})
	}
}

func newHandlerWithBalances(t *testing.T, balances map[int64]int64) (*Handler, *service.Service) {
	t.Helper()
	svc := service.NewService(local.NewMapStorage())
	for userID, balance := range balances {
		if _, err := svc.Deposit(context.Background(), userID, balance); err != nil {
			t.Fatalf("prepare balance for user %d: %v", userID, err)
		}
	}
	return NewHandler(svc), svc
}

func requireStoredBalance(t *testing.T, svc *service.Service, userID, wantBalance int64) {
	t.Helper()
	gotBalance, err := svc.Balance(context.Background(), userID)
	if err != nil {
		t.Fatalf("read stored balance for user %d: %v", userID, err)
	}
	if gotBalance != wantBalance {
		t.Errorf("user %d: expected stored balance %d, got %d", userID, wantBalance, gotBalance)
	}
}

func requireGRPCError(t *testing.T, err error, wantCode codes.Code, wantMessage string) {
	t.Helper()
	if gotCode := status.Code(err); gotCode != wantCode {
		t.Fatalf("expected code %v, got %v (error: %v)", wantCode, gotCode, err)
	}
	if gotMessage := status.Convert(err).Message(); gotMessage != wantMessage {
		t.Errorf("expected message %q, got %q", wantMessage, gotMessage)
	}
}
