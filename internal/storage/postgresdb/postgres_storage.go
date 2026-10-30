package postgresdb

import (
	"context"
	"errors"

	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const codeNumericOutOfRange = "22003"
const balanceNonNegative = "balance_non_negative"

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

func (s *PostgresStorage) Deposit(userID, amount int64) (int64, error) {
	var balance int64
	ctx := context.Background()
	err := s.pool.QueryRow(ctx, `
								insert into user_balance (user_id, balance) values (
									$1,
									$2
								)
								on conflict (user_id)
								do update set 
									balance = user_balance.balance + EXCLUDED.balance
								RETURNING balance;`,
		userID, amount).Scan(&balance)
	var pgErr *pgconn.PgError
	if err != nil {
		if errors.As(err, &pgErr) {
			if pgErr.Code == codeNumericOutOfRange {
				return 0, domain.ErrBalanceOverflow
			}
		}
		return 0, err
	}
	return balance, nil
}

func (s *PostgresStorage) Withdraw(userID, amount int64) (int64, error) {
	var balance int64
	ctx := context.Background()
	err := s.pool.QueryRow(ctx, `
								update user_balance set 
									balance = user_balance.balance - $2
								where user_id = $1
								RETURNING balance;`,
		userID, amount).Scan(&balance)
	var pgErr *pgconn.PgError
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrNotFound
		} else if errors.As(err, &pgErr) {
			if pgErr.ConstraintName == balanceNonNegative {
				return 0, domain.ErrInsufficientFunds
			}
		}
		return 0, err
	}
	return balance, nil
}
