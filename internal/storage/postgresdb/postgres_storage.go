package postgresdb

import (
	"context"
	"errors"

	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStorage struct {
	pool *pgxpool.Pool
}

func NewPostgresStorage(pool *pgxpool.Pool) *PostgresStorage {
	return &PostgresStorage{
		pool: pool,
	}
}

func (s *PostgresStorage) Balance(userID int64) (int64, error) {
	var balance int64
	ctx := context.Background()
	err := s.pool.QueryRow(ctx, "SELECT balance FROM user_balance WHERE user_id = $1", userID).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		} else {
			return 0, err
		}
	}
	return balance, nil
}
