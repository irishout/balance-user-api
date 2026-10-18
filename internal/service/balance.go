package service

import (
	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/irishout/balance-user-api/internal/storage/local"
)

type Service struct {
	storage *local.MapStorage
}

func NewService(s *local.MapStorage) *Service {
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
