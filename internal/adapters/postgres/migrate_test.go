package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/ynov/equipment-loans/internal/adapters/postgres"
)

func TestMigrateIsIdempotentAndConcurrencySafe(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is empty")
	}
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS loan, item, equipment_model, schema_migrations CASCADE"); err != nil {
		t.Fatal(err)
	}

	errs := runConcurrently(4, func(int) error { return postgres.Migrate(ctx, pool) })
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent migrate: %v", err)
		}
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var applied, models, items int
	row := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM schema_migrations), (SELECT count(*) FROM equipment_model), (SELECT count(*) FROM item)")
	if err := row.Scan(&applied, &models, &items); err != nil {
		t.Fatal(err)
	}
	if applied != 2 || models != 3 || items != 8 {
		t.Fatalf("applied=%d models=%d items=%d", applied, models, items)
	}
}
