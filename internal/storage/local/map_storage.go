package local

import (
	"math"
	"sync"

	"github.com/irishout/balance-user-api/internal/domain"
)

type MapStorage struct {
	balances map[int64]int64
	mu       sync.Mutex
}

func NewMapStorage() *MapStorage {
	return &MapStorage{
		balances: make(map[int64]int64),
	}
}

func (s *MapStorage) Balance(userID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance, ok := s.balances[userID]
	if !ok {
		return 0, domain.ErrNotFound
	}
	return balance, nil
}

func (s *MapStorage) Deposit(userID, amount int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance := s.balances[userID]

	if balance > math.MaxInt64-amount {
		return 0, domain.ErrBalanceOverflow
	}

	s.balances[userID] += amount
	return balance + amount, nil
}

func (s *MapStorage) Withdraw(userID, amount int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance, ok := s.balances[userID]
	if !ok {
		return 0, domain.ErrNotFound
	}

	if balance < amount {
		return 0, domain.ErrInsufficientFunds
	}

	s.balances[userID] -= amount
	return balance - amount, nil
}
