package service

import (
	"context"

	"github.com/irishout/balance-user-api/internal/domain"
)

type Storage interface {
	Balance(ctx context.Context, userID int64) (int64, error)
	Deposit(ctx context.Context, userID, amount int64) (int64, error)
	Withdraw(ctx context.Context, userID, amount int64) (int64, error)
	Transfer(ctx context.Context, fromID, toID, amount int64) (int64, int64, error)
}

type Service struct {
	storage Storage
}

func NewService(s Storage) *Service {
	return &Service{
		storage: s,
	}
}

func (s *Service) Deposit(ctx context.Context, userID, amount int64) (int64, error) {
	if userID <= 0 {
		return 0, domain.ErrInvalidUserID
	}

	if amount <= 0 {
		return 0, domain.ErrInvalidAmount
	}

	return s.storage.Deposit(ctx, userID, amount)
}

func (s *Service) Withdraw(ctx context.Context, userID, amount int64) (int64, error) {
	if userID <= 0 {
		return 0, domain.ErrInvalidUserID
	}

	if amount <= 0 {
		return 0, domain.ErrInvalidAmount
	}

	return s.storage.Withdraw(ctx, userID, amount)
}

func (s *Service) Transfer(ctx context.Context, fromID, toID, amount int64) (int64, int64, error) {
	if fromID <= 0 {
		return 0, 0, domain.ErrInvalidSenderID
	}

	if toID <= 0 {
		return 0, 0, domain.ErrInvalidRecipientID
	}

	if fromID == toID {
		return 0, 0, domain.ErrSelfTransfer
	}

	if amount <= 0 {
		return 0, 0, domain.ErrInvalidAmount
	}

	return s.storage.Transfer(ctx, fromID, toID, amount)
}

func (s *Service) Balance(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, domain.ErrInvalidUserID
	}

	return s.storage.Balance(ctx, userID)
}
