package postgresdb

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestPostgresStorage(t *testing.T, ctx context.Context) *PostgresStorage {
	t.Helper()
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	if db := config.ConnConfig.Database; db != "balance_test" {
		t.Fatalf("need balance_test database, got %q", db)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE user_balance, operations"); err != nil {
		t.Fatalf("reset test database (apply migrations 001 and 002): %v", err)
	}
	return NewPostgresStorage(pool)
}

func TestTransferRollbackRecipientNotFound(t *testing.T) {
	ctx := context.Background()
	storage := newTestPostgresStorage(t, ctx)

	_, err := storage.Deposit(ctx, 1, 1000, "")
	if err != nil {
		t.Fatalf("storage Deposit error %v", err)
	}

	_, _, err = storage.Transfer(ctx, 1, 2, 300, "")
	if !errors.Is(err, domain.ErrRecipientNotFound) {
		t.Fatalf("Expected error: %v, got: %v", domain.ErrRecipientNotFound, err)
	}
	senderBalance, err := storage.Balance(ctx, 1)
	if err != nil {
		t.Fatalf("storage Balance error %v", err)
	}
	if senderBalance != 1000 {
		t.Fatalf("Sender balance must be 1000, got: %v", senderBalance)
	}

}

func TestWithdrawConcurrent(t *testing.T) {
	ctx := context.Background()
	storage := newTestPostgresStorage(t, ctx)

	_, err := storage.Deposit(ctx, 1, 100000, "")
	if err != nil {
		t.Fatalf("storage Deposit error %v", err)
	}

	var wg sync.WaitGroup
	errorsSlice := make([]error, 200)

	for i := 0; i < len(errorsSlice); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := storage.Withdraw(ctx, 1, 1000, "")
			errorsSlice[i] = err
		}()
	}
	wg.Wait()

	sucsessCount := 0
	insufficientFundsCount := 0
	for _, v := range errorsSlice {
		if errors.Is(v, nil) {
			sucsessCount++
		} else if errors.Is(v, domain.ErrInsufficientFunds) {
			insufficientFundsCount++
		} else {
			t.Errorf("Unexpected result. Expected nil or %v. Got: %v", domain.ErrInsufficientFunds, v)
		}
	}

	if !(sucsessCount == 100 && insufficientFundsCount == 100) {
		t.Fatalf("Expexted 100 sucsesses and 100 insufficient funds errors. Got: %v sucsessWithdrawCount and %v insufficient funds",
			sucsessCount, insufficientFundsCount)
	}

	balance, err := storage.Balance(ctx, 1)
	if err != nil {
		t.Fatalf("storage Balance error %v", err)
	}
	if balance != 0 {
		t.Fatalf("Sender balance must be 0, got: %v", balance)
	}
}

func TestTransferConcurrentOppositeDirections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	storage := newTestPostgresStorage(t, ctx)

	_, err := storage.Deposit(ctx, 1, 1000, "")
	if err != nil {
		t.Fatalf("storage Deposit error %v", err)
	}

	_, err = storage.Deposit(ctx, 2, 1000, "")
	if err != nil {
		t.Fatalf("storage Deposit error %v", err)
	}

	var wg sync.WaitGroup
	errorsSlice := make([]error, 200)

	for i := 0; i < len(errorsSlice)/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err1 := storage.Transfer(ctx, 1, 2, 1, "")
			errorsSlice[i] = err1
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err2 := storage.Transfer(ctx, 2, 1, 1, "")
			errorsSlice[100+i] = err2
		}()
	}
	wg.Wait()

	sucsessCount := 0
	for _, v := range errorsSlice {
		if v == nil {
			sucsessCount++
		} else {
			t.Errorf("Unexpected result. Expected nil. Got: %v", v)
		}
	}

	if !(sucsessCount == 200) {
		t.Fatalf("Expexted 200 sucsesses. Got: %v sucsessCount ",
			sucsessCount)
	}

	balance1ID, err := storage.Balance(ctx, 1)
	if err != nil {
		t.Fatalf("storage Balance error %v", err)
	}
	if balance1ID != 1000 {
		t.Fatalf("1 ID balance must be 1000, got: %v", balance1ID)
	}

	balance2ID, err := storage.Balance(ctx, 2)
	if err != nil {
		t.Fatalf("storage Balance error %v", err)
	}
	if balance2ID != 1000 {
		t.Fatalf("2 ID balance must be 1000, got: %v", balance2ID)
	}
}

func TestListOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	storage := newTestPostgresStorage(t, ctx)

	if _, err := storage.Deposit(ctx, 1, 1000, ""); err != nil {
		t.Fatalf("deposit for sender: %v", err)
	}
	if _, err := storage.Deposit(ctx, 2, 200, ""); err != nil {
		t.Fatalf("deposit for recipient: %v", err)
	}
	if _, _, err := storage.Transfer(ctx, 1, 2, 300, ""); err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if _, err := storage.Withdraw(ctx, 2, 300, ""); err != nil {
		t.Fatalf("withdraw: %v", err)
	}

	// Give the deposits a newer timestamp and make the transfer and withdrawal tie.
	if _, err := storage.pool.Exec(ctx, `
		UPDATE operations
		SET operation_date = CASE operation_type
		        WHEN 'deposit' THEN TIMESTAMPTZ '2026-01-02 00:00:00+00'
		        ELSE TIMESTAMPTZ '2026-01-01 00:00:00+00'
		    END,
		    comment = CASE WHEN operation_type = 'transfer' THEN NULL ELSE comment END`); err != nil {
		t.Fatalf("set history fixtures: %v", err)
	}

	history, err := storage.ListOperations(ctx, 2, 10, 0, "date")
	if err != nil {
		t.Fatalf("list recipient history: %v", err)
	}
	if len(history) != 3 || history[0].Type != "deposit" || history[1].Type != "withdraw" || history[2].Type != "transfer" {
		t.Fatalf("got %+v, want deposit, withdrawal, transfer", history)
	}
	if history[0].Amount != 200 || history[1].Amount != 300 || history[0].RecipientID != nil || history[1].RecipientID != nil {
		t.Fatalf("unexpected deposit or withdrawal: %+v", history[:2])
	}
	transfer := history[2]
	if transfer.UserID != 1 || transfer.RecipientID == nil || *transfer.RecipientID != 2 || transfer.Amount != 300 || transfer.Comment != "" {
		t.Fatalf("unexpected incoming transfer: %+v", transfer)
	}

	senderHistory, err := storage.ListOperations(ctx, 1, 10, 0, "date")
	if err != nil || len(senderHistory) != 2 || senderHistory[1].ID != transfer.ID {
		t.Fatalf("sender history = %+v, error = %v; want the same transfer", senderHistory, err)
	}

	page, err := storage.ListOperations(ctx, 2, 1, 1, "amount")
	if err != nil || len(page) != 1 || page[0].ID != transfer.ID {
		t.Fatalf("page = %+v, error = %v; want only transfer %d", page, err, transfer.ID)
	}

	empty, err := storage.ListOperations(ctx, 3, 10, 0, "")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("history for unknown user = %#v, error = %v; want an empty slice", empty, err)
	}
}

func TestBalanceChangesRollbackWhenHistoryInsertFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	fixture := newTestPostgresStorage(t, ctx)
	config := fixture.pool.Config()
	// The temporary operations table belongs to one PostgreSQL connection.
	config.MaxConns = 1
	config.MinConns = 0
	config.MinIdleConns = 0
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open rollback test pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "CREATE TEMP TABLE operations (LIKE operations INCLUDING ALL)"); err != nil {
		t.Fatalf("create temporary history table: %v", err)
	}
	storage := NewPostgresStorage(pool)

	if _, err := storage.Deposit(ctx, 1, 1000, ""); err != nil {
		t.Fatalf("seed sender balance: %v", err)
	}
	if _, err := storage.Deposit(ctx, 2, 500, ""); err != nil {
		t.Fatalf("seed recipient balance: %v", err)
	}

	// Reject a valid balance operation specifically at the history insert.
	if _, err := pool.Exec(ctx, `
		ALTER TABLE pg_temp.operations ADD CONSTRAINT test_reject_history_amount CHECK (amount <> 300)`); err != nil {
		t.Fatalf("add test constraint: %v", err)
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{"deposit", func() error {
			_, err := storage.Deposit(ctx, 1, 300, "")
			return err
		}},
		{"withdraw", func() error {
			_, err := storage.Withdraw(ctx, 1, 300, "")
			return err
		}},
		{"transfer", func() error {
			_, _, err := storage.Transfer(ctx, 1, 2, 300, "")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.run()
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.ConstraintName != "test_reject_history_amount" {
				t.Fatalf("error = %v, want the history constraint violation", err)
			}
			for userID, want := range map[int64]int64{1: 1000, 2: 500} {
				balance, err := storage.Balance(ctx, userID)
				if err != nil || balance != want {
					t.Fatalf("user %d balance = %d, error = %v; want %d", userID, balance, err, want)
				}
			}
			var count int
			err = pool.QueryRow(ctx, "SELECT count(*) FROM pg_temp.operations").Scan(&count)
			if err != nil || count != 2 {
				t.Fatalf("history count = %d, error = %v; want two seed deposits", count, err)
			}
		})
	}
}
