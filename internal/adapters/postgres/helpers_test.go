package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ynov/equipment-loans/internal/adapters/postgres"
	"github.com/ynov/equipment-loans/internal/domain"
)

const modelID = "11111111-1111-4111-8111-111111111111"

func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is empty")
	}
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE loan, item, equipment_model CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO equipment_model (id, name, category, max_loan_days) VALUES ($1, 'Raspberry Pi 5', 'sbc', 7)", modelID); err != nil {
		t.Fatalf("seed model: %v", err)
	}
	return pool
}

func seedItem(t *testing.T, pool *pgxpool.Pool, number string, status domain.ItemStatus) domain.ItemID {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(), "INSERT INTO item (id, model_id, inventory_number, status) VALUES ($1, $2, $3, $4)", id, modelID, number, string(status))
	if err != nil {
		t.Fatalf("seed item: %v", err)
	}
	return domain.ItemID(id)
}

func newLoan(item domain.ItemID, sub string, borrowedAt time.Time) domain.Loan {
	return domain.Loan{
		ID:          domain.LoanID(uuid.NewString()),
		ItemID:      item,
		BorrowerSub: sub,
		BorrowedAt:  borrowedAt,
		DueAt:       borrowedAt.Add(7 * 24 * time.Hour),
	}
}
