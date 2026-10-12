package local

import (
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
