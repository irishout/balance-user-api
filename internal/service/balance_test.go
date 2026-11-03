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
}

func (s *storageMock) Deposit(ctx context.Context, userID, amount int64) (int64, error) {
	s.receivedUserID = userID
	s.receivedAmount = amount
	return 1500, s.depositErr
}
func (s *storageMock) Withdraw(ctx context.Context, userID, amount int64) (int64, error) {
	return 1500, nil
}
func (s *storageMock) Transfer(ctx context.Context, fromID, toID, amount int64) (int64, int64, error) {
	return 1500, 1000, nil
}
func (s *storageMock) Balance(ctx context.Context, userID int64) (int64, error) {
	return 1500, nil
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
