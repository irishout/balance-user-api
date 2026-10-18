package service

import (
	"github.com/irishout/balance-user-api/internal/domain"
)

type Storage interface {
	Balance(userID int64) (int64, error)
	Deposit(userID, amount int64) (int64, error)
	Withdraw(userID, amount int64) (int64, error)
	Transfer(fromID, toID, amount int64) (int64, int64, error)
}

type Service struct {
	storage Storage
}

func NewService(s Storage) *Service {
	return &Service{
		storage: s,
	}
}

func (s *Service) Deposit(userID, amount int64) (int64, error) {
	if userID <= 0 {
		return 0, domain.ErrInvalidUserID
	}

	if amount <= 0 {
		return 0, domain.ErrInvalidAmount
	}

	return s.storage.Deposit(userID, amount)
}
