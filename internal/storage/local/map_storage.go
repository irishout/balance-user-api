package local

import (
	"context"
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

func (s *MapStorage) Balance(ctx context.Context, userID int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance, ok := s.balances[userID]
	if !ok {
		return 0, domain.ErrNotFound
	}
	return balance, nil
}

func (s *MapStorage) Deposit(ctx context.Context, userID, amount int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	balance := s.balances[userID]

	if balance > math.MaxInt64-amount {
		return 0, domain.ErrBalanceOverflow
	}

	s.balances[userID] += amount
	return balance + amount, nil
}

func (s *MapStorage) Withdraw(ctx context.Context, userID, amount int64) (int64, error) {
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

func (s *MapStorage) Transfer(ctx context.Context, fromID, toID, amount int64) (int64, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fromBalance, ok_from := s.balances[fromID]
	if !ok_from {
		return 0, 0, domain.ErrSenderNotFound
	}

	toBalance, ok_to := s.balances[toID]
	if !ok_to {
		return 0, 0, domain.ErrRecipientNotFound
	}

	if fromBalance < amount {
		return 0, 0, domain.ErrInsufficientFunds
	}

	if toBalance > math.MaxInt64-amount {
		return 0, 0, domain.ErrBalanceOverflow
	}

	s.balances[fromID] -= amount
	s.balances[toID] += amount
	return fromBalance - amount, toBalance + amount, nil
}
