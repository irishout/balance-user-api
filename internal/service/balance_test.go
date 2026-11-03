package service

import (
	"context"
	"errors"
	"testing"

	"github.com/irishout/balance-user-api/internal/domain"
)

type storageMock struct {
	receivedUserID int64
	receivedAmount int64
	depositErr     error
	withdrawErr    error
	withdrawCalls  int
	balanceResult  int64
	balanceErr     error
	balanceCalls   int
	receivedFromID int64
	receivedToID   int64
	transferErr    error
	transferCalls  int
}

func (s *storageMock) Deposit(ctx context.Context, userID, amount int64) (int64, error) {
	s.receivedUserID = userID
	s.receivedAmount = amount
	return 1500, s.depositErr
}
func (s *storageMock) Withdraw(ctx context.Context, userID, amount int64) (int64, error) {
	s.withdrawCalls++
	s.receivedUserID = userID
	s.receivedAmount = amount
	return 1500, s.withdrawErr
}
func (s *storageMock) Transfer(ctx context.Context, fromID, toID, amount int64) (int64, int64, error) {
	s.transferCalls++
	s.receivedFromID = fromID
	s.receivedToID = toID
	s.receivedAmount = amount
	return 1500, 1000, s.transferErr
}
func (s *storageMock) Balance(ctx context.Context, userID int64) (int64, error) {
	s.balanceCalls++
	s.receivedUserID = userID
	return s.balanceResult, s.balanceErr
}

func TestDepositSuccess(t *testing.T) {
	storage := &storageMock{}
	svc := NewService(storage)

	balance, err := svc.Deposit(context.Background(), 1, 1000)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if balance != 1500 {
		t.Fatalf("expected balance 1500, got %d", balance)
	}
	if storage.receivedUserID != 1 {
		t.Errorf("expected storage to receive user ID 1, got %d", storage.receivedUserID)
	}
	if storage.receivedAmount != 1000 {
		t.Errorf("expected storage to receive amount 1000, got %d", storage.receivedAmount)
	}
}

func TestDepositStorageError(t *testing.T) {
	wantErr := errors.New("storage unavailable")
	storage := &storageMock{depositErr: wantErr}
	svc := NewService(storage)

	_, err := svc.Deposit(context.Background(), 1, 1000)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected storage error %v, got %v", wantErr, err)
	}
}

func TestDepositInvalidAmount(t *testing.T) {
	type errType struct {
		name    string
		amount  int64
		wantErr error
	}
	var errTable []errType
	errTable = append(errTable, errType{
		name:    "negative sum",
		amount:  -100,
		wantErr: domain.ErrInvalidAmount,
	}, errType{
		name:    "zero sum",
		amount:  0,
		wantErr: domain.ErrInvalidAmount,
	})

	svc := NewService(nil)
	for _, tc := range errTable {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Deposit(context.Background(), 1, tc.amount)

			if !errors.Is(err, tc.wantErr) {
				t.Fatal("Case:", tc.name, "Expected:", tc.wantErr, "Got:", err)
			}
		})
	}
}
func TestDepositInvalidUserID(t *testing.T) {
	type errType struct {
		name    string
		ID      int64
		wantErr error
	}
	var errTable []errType
	errTable = append(errTable, errType{
		name:    "negative ID",
		ID:      -1,
		wantErr: domain.ErrInvalidUserID,
	}, errType{
		name:    "zero ID",
		ID:      0,
		wantErr: domain.ErrInvalidUserID,
	})

	svc := NewService(nil)
	for _, tc := range errTable {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Deposit(context.Background(), tc.ID, 100)

			if !errors.Is(err, tc.wantErr) {
				t.Fatal("Case:", tc.name, "Expected:", tc.wantErr, "Got:", err)
			}
		})
	}
}

func TestWithdrawSuccess(t *testing.T) {
	storage := &storageMock{}
	svc := NewService(storage)

	balance, err := svc.Withdraw(context.Background(), 1, 1000)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if balance != 1500 {
		t.Errorf("expected balance 1500, got %d", balance)
	}
	if storage.withdrawCalls != 1 {
		t.Errorf("expected storage Withdraw to be called once, got %d calls", storage.withdrawCalls)
	}
	if storage.receivedUserID != 1 {
		t.Errorf("expected storage to receive user ID 1, got %d", storage.receivedUserID)
	}
	if storage.receivedAmount != 1000 {
		t.Errorf("expected storage to receive amount 1000, got %d", storage.receivedAmount)
	}
}

func TestWithdrawStorageError(t *testing.T) {
	tests := []struct {
		name    string
		wantErr error
	}{
		{name: "user not found", wantErr: domain.ErrNotFound},
		{name: "insufficient funds", wantErr: domain.ErrInsufficientFunds},
		{name: "unknown storage error", wantErr: errors.New("storage unavailable")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storage := &storageMock{withdrawErr: tc.wantErr}
			svc := NewService(storage)

			_, err := svc.Withdraw(context.Background(), 1, 1000)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected storage error %v, got %v", tc.wantErr, err)
			}
			if storage.withdrawCalls != 1 {
				t.Errorf("expected storage Withdraw to be called once, got %d calls", storage.withdrawCalls)
			}
		})
	}
}

func TestWithdrawInvalidAmount(t *testing.T) {
	tests := []struct {
		name   string
		amount int64
	}{
		{name: "negative sum", amount: -100},
		{name: "zero sum", amount: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil)
			_, err := svc.Withdraw(context.Background(), 1, tc.amount)
			if !errors.Is(err, domain.ErrInvalidAmount) {
				t.Fatalf("expected %v, got %v", domain.ErrInvalidAmount, err)
			}
		})
	}
}

func TestWithdrawInvalidUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
	}{
		{name: "negative ID", userID: -1},
		{name: "zero ID", userID: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil)
			_, err := svc.Withdraw(context.Background(), tc.userID, 100)
			if !errors.Is(err, domain.ErrInvalidUserID) {
				t.Fatalf("expected %v, got %v", domain.ErrInvalidUserID, err)
			}
		})
	}
}

func TestTransferSuccess(t *testing.T) {
	storage := &storageMock{}
	svc := NewService(storage)

	fromBalance, toBalance, err := svc.Transfer(context.Background(), 1, 2, 500)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if fromBalance != 1500 {
		t.Errorf("expected sender balance 1500, got %d", fromBalance)
	}
	if toBalance != 1000 {
		t.Errorf("expected recipient balance 1000, got %d", toBalance)
	}
	if storage.transferCalls != 1 {
		t.Errorf("expected storage Transfer to be called once, got %d calls", storage.transferCalls)
	}
	if storage.receivedFromID != 1 {
		t.Errorf("expected storage to receive sender ID 1, got %d", storage.receivedFromID)
	}
	if storage.receivedToID != 2 {
		t.Errorf("expected storage to receive recipient ID 2, got %d", storage.receivedToID)
	}
	if storage.receivedAmount != 500 {
		t.Errorf("expected storage to receive amount 500, got %d", storage.receivedAmount)
	}
}

func TestTransferStorageError(t *testing.T) {
	tests := []struct {
		name    string
		wantErr error
	}{
		{name: "sender not found", wantErr: domain.ErrSenderNotFound},
		{name: "recipient not found", wantErr: domain.ErrRecipientNotFound},
		{name: "insufficient funds", wantErr: domain.ErrInsufficientFunds},
		{name: "balance overflow", wantErr: domain.ErrBalanceOverflow},
		{name: "unknown storage error", wantErr: errors.New("storage unavailable")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storage := &storageMock{transferErr: tc.wantErr}
			svc := NewService(storage)

			_, _, err := svc.Transfer(context.Background(), 1, 2, 500)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected storage error %v, got %v", tc.wantErr, err)
			}
			if storage.transferCalls != 1 {
				t.Errorf("expected storage Transfer to be called once, got %d calls", storage.transferCalls)
			}
		})
	}
}

func TestTransferInvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		fromID  int64
		toID    int64
		amount  int64
		wantErr error
	}{
		{name: "negative sender ID", fromID: -1, toID: 2, amount: 500, wantErr: domain.ErrInvalidSenderID},
		{name: "zero sender ID", fromID: 0, toID: 2, amount: 500, wantErr: domain.ErrInvalidSenderID},
		{name: "negative recipient ID", fromID: 1, toID: -1, amount: 500, wantErr: domain.ErrInvalidRecipientID},
		{name: "zero recipient ID", fromID: 1, toID: 0, amount: 500, wantErr: domain.ErrInvalidRecipientID},
		{name: "self transfer", fromID: 1, toID: 1, amount: 500, wantErr: domain.ErrSelfTransfer},
		{name: "negative amount", fromID: 1, toID: 2, amount: -100, wantErr: domain.ErrInvalidAmount},
		{name: "zero amount", fromID: 1, toID: 2, amount: 0, wantErr: domain.ErrInvalidAmount},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil)
			_, _, err := svc.Transfer(context.Background(), tc.fromID, tc.toID, tc.amount)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestBalanceSuccess(t *testing.T) {
	tests := []struct {
		name    string
		balance int64
	}{
		{name: "positive balance", balance: 1500},
		{name: "zero balance", balance: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storage := &storageMock{balanceResult: tc.balance}
			svc := NewService(storage)

			balance, err := svc.Balance(context.Background(), 7)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if balance != tc.balance {
				t.Errorf("expected balance %d, got %d", tc.balance, balance)
			}
			if storage.balanceCalls != 1 {
				t.Errorf("expected storage Balance to be called once, got %d calls", storage.balanceCalls)
			}
			if storage.receivedUserID != 7 {
				t.Errorf("expected storage to receive user ID 7, got %d", storage.receivedUserID)
			}
		})
	}
}

func TestBalanceStorageError(t *testing.T) {
	tests := []struct {
		name    string
		wantErr error
	}{
		{name: "user not found", wantErr: domain.ErrNotFound},
		{name: "unknown storage error", wantErr: errors.New("storage unavailable")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			storage := &storageMock{balanceErr: tc.wantErr}
			svc := NewService(storage)

			_, err := svc.Balance(context.Background(), 7)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected storage error %v, got %v", tc.wantErr, err)
			}
			if storage.balanceCalls != 1 {
				t.Errorf("expected storage Balance to be called once, got %d calls", storage.balanceCalls)
			}
		})
	}
}

func TestBalanceInvalidUserID(t *testing.T) {
	tests := []struct {
		name   string
		userID int64
	}{
		{name: "negative ID", userID: -1},
		{name: "zero ID", userID: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(nil)
			_, err := svc.Balance(context.Background(), tc.userID)
			if !errors.Is(err, domain.ErrInvalidUserID) {
				t.Fatalf("expected %v, got %v", domain.ErrInvalidUserID, err)
			}
		})
	}
}
