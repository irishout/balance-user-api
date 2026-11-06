package postgresdb

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/irishout/balance-user-api/internal/domain"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTransferRollbackRecipientNotFound(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("TEST_DATABASE is not available. %v", err)
	}
	defer pool.Close()

	err = pool.Ping(ctx)
	if err != nil {
		t.Fatalf("TEST_DATABASE is not available. %v", err)
	}
	storage := NewPostgresStorage(pool)

	if db := pool.Config().ConnConfig.Database; db != "balance_test" {
		t.Fatalf("Need balance_test database, got %v database", db)
	}
	_, err = pool.Exec(ctx, "TRUNCATE TABLE user_balance")
	if err != nil {
		t.Fatalf("Couldn`t delete old data from test DB. %v", err)
	}

	_, err = storage.Deposit(ctx, 1, 1000)
	if err != nil {
		t.Fatalf("storage Deposit error %v", err)
	}

	_, _, err = storage.Transfer(ctx, 1, 2, 300)
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
