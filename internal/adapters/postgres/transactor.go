package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ynov/equipment-loans/internal/ports"
)

type Transactor struct {
	pool *pgxpool.Pool
}

func NewTransactor(pool *pgxpool.Pool) *Transactor {
	return &Transactor{pool: pool}
}

func (t *Transactor) InTx(ctx context.Context, fn func(ctx context.Context, repos ports.Repositories) error) error {
	tx, err := t.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: begin: %w", err)
	}
	defer tx.Rollback(context.WithoutCancel(ctx))
	repos := ports.Repositories{Items: &ItemRepository{db: tx}, Loans: &LoanRepository{db: tx}}
	if err := fn(ctx, repos); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit: %w", err)
	}
	return nil
}

func NewItemRepository(pool *pgxpool.Pool) *ItemRepository {
	return &ItemRepository{db: pool}
}

func NewLoanRepository(pool *pgxpool.Pool) *LoanRepository {
	return &LoanRepository{db: pool}
}
