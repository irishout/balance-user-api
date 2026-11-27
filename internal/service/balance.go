package service

import (
	"context"

	"github.com/irishout/balance-user-api/internal/domain"
)

type Storage interface {
	Balance(ctx context.Context, userID int64) (int64, error)
	Deposit(ctx context.Context, userID, amount int64, comment string) (int64, error)
	Withdraw(ctx context.Context, userID, amount int64, comment string) (int64, error)
	Transfer(ctx context.Context, fromID, toID, amount int64, comment string) (int64, int64, error)
	ListOperations(ctx context.Context, userID int64, limit, offset int, sortBy string) ([]domain.Operation, error)
}

type Service struct {
	storage Storage
}

func NewService(s Storage) *Service {
	return &Service{
		storage: s,
	}
}

func (s *Service) Deposit(ctx context.Context, userID, amount int64, comment string) (int64, error) {
	if userID <= 0 {
		return 0, domain.ErrInvalidUserID
	}

	if amount <= 0 {
		return 0, domain.ErrInvalidAmount
	}

	return s.storage.Deposit(ctx, userID, amount, comment)
}

func (s *Service) Withdraw(ctx context.Context, userID, amount int64, comment string) (int64, error) {
	if userID <= 0 {
		return 0, domain.ErrInvalidUserID
	}

	if amount <= 0 {
		return 0, domain.ErrInvalidAmount
	}

	return s.storage.Withdraw(ctx, userID, amount, comment)
}

func (s *Service) Transfer(ctx context.Context, fromID, toID, amount int64, comment string) (int64, int64, error) {
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

	return s.storage.Transfer(ctx, fromID, toID, amount, comment)
}

func (s *Service) Balance(ctx context.Context, userID int64) (int64, error) {
	if userID <= 0 {
		return 0, domain.ErrInvalidUserID
	}

	return s.storage.Balance(ctx, userID)
}

func (s *Service) ListOperations(ctx context.Context, userID int64, limit, offset int, sortBy string) ([]domain.Operation, error) {
	if userID <= 0 {
		return nil, domain.ErrInvalidUserID
	}
	if limit < 0 || offset < 0 {
		return nil, domain.ErrInvalidPagination
	}
	if limit == 0 {
		limit = 20
	}

	switch sortBy {
	case "":
		sortBy = "date"
	case "date", "amount":
	default:
		return nil, domain.ErrInvalidOperationSort
	}

	return s.storage.ListOperations(ctx, userID, limit, offset, sortBy)
}
