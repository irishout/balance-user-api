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

func (s *PostgresStorage) ListOperations(ctx context.Context, userID int64, limit, offset int, sortBy string) ([]domain.Operation, error) {
	var sortColumn string
	switch sortBy {
	case "", "date":
		sortColumn = "operation_date"
	case "amount":
		sortColumn = "amount"
	default:
		return nil, domain.ErrInvalidOperationSort
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, operation_type, operation_date, user_id, recipient_id, amount,
		       COALESCE(comment, '') AS comment
		FROM operations
		WHERE user_id = $1 OR recipient_id = $1
		ORDER BY `+sortColumn+` DESC, id DESC
		LIMIT $2 OFFSET $3;`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	history := make([]domain.Operation, 0)

	for rows.Next() {
		var row domain.Operation
		if err := rows.Scan(&row.ID, &row.Type, &row.Date, &row.UserID, &row.RecipientID, &row.Amount, &row.Comment); err != nil {
			return nil, err
		}
		history = append(history, row)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return history, nil
}

func (s *PostgresStorage) Balance(ctx context.Context, userID int64) (int64, error) {
	var balance int64
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

func (s *PostgresStorage) Deposit(ctx context.Context, userID, amount int64, comment string) (int64, error) {
	var balance int64
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
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

	_, err = tx.Exec(ctx, `
		INSERT INTO operations (operation_type, user_id, amount, comment)
		VALUES ('deposit', $1, $2, $3)`, userID, amount, comment)

	if err != nil {
		return 0, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, err
	}

	return balance, nil
}

func (s *PostgresStorage) Withdraw(ctx context.Context, userID, amount int64, comment string) (int64, error) {
	var balance int64
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	err = tx.QueryRow(ctx, `
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

	_, err = tx.Exec(ctx, `
		INSERT INTO operations (operation_type, user_id, amount, comment)
		VALUES ('withdraw', $1, $2, $3)`, userID, amount, comment)

	if err != nil {
		return 0, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return 0, err
	}

	return balance, nil
}

func (s *PostgresStorage) Transfer(ctx context.Context, fromID, toID, amount int64, comment string) (int64, int64, error) {
	var balanceFrom, balanceTo int64
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, "select user_id from user_balance where user_id in ($1, $2) order by user_id for update;", fromID, toID)
	if err != nil {
		return 0, 0, err
	}
	err = tx.QueryRow(ctx, `
								update user_balance set 
									balance = user_balance.balance - $2
								where user_id = $1
								RETURNING balance;`,
		fromID, amount).Scan(&balanceFrom)

	var pgErr *pgconn.PgError
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, domain.ErrSenderNotFound
		} else if errors.As(err, &pgErr) {
			if pgErr.ConstraintName == balanceNonNegative {
				return 0, 0, domain.ErrInsufficientFunds
			}
		}
		return 0, 0, err
	}
	err = tx.QueryRow(ctx, `
								update user_balance set 
									balance = user_balance.balance + $2
								where user_id = $1
								RETURNING balance;`,
		toID, amount).Scan(&balanceTo)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, 0, domain.ErrRecipientNotFound
		} else if errors.As(err, &pgErr) {
			if pgErr.Code == codeNumericOutOfRange {
				return 0, 0, domain.ErrBalanceOverflow
			}
		}
		return 0, 0, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO operations (operation_type, user_id, recipient_id, amount, comment)
		VALUES ('transfer', $1, $2, $3, $4)`, fromID, toID, amount, comment)

	if err != nil {
		return 0, 0, err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return 0, 0, err
	}
	return balanceFrom, balanceTo, nil
}
