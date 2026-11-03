package service

import (
	"context"
	"errors"
	"testing"

	"github.com/irishout/balance-user-api/internal/domain"
)

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
				t.Fatal("Test:", tc.name, "Expected:", tc.wantErr, "Got:", err)
			}
		})
	}
}
