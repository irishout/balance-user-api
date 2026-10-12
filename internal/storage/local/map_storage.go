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

func (s *MapStorage) Transfer(user_from_ID, user_to_ID, amount int64) (int64, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance_from, ok_from := s.balances[user_from_ID]
	if !ok_from {
		return 0, 0, domain.ErrSenderNotFound
	}

	balance_to, ok_to := s.balances[user_to_ID]
	if !ok_to {
		return 0, 0, domain.ErrRecipientNotFound
	}

	if balance_from < amount {
		return 0, 0, domain.ErrInsufficientFunds
	}

	if balance_to > math.MaxInt64-amount {
		return 0, 0, domain.ErrBalanceOverflow
	}

	s.balances[user_from_ID] -= amount
	s.balances[user_to_ID] += amount
	return balance_from - amount, balance_to + amount, nil
}
