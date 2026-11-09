package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ynov/equipment-loans/internal/adapters/postgres"
	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/ports"
)

const workers = 12

func runConcurrently(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = fn(i)
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

func countOutcomes(t *testing.T, errs []error, want error) (successes, rejected int) {
	t.Helper()
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, want):
			rejected++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return successes, rejected
}

func TestRepo_UniqueIndexAllowsSingleActiveLoan(t *testing.T) {
	pool := newPool(t)
	item := seedItem(t, pool, "INV-1", domain.ItemAvailable)
	tx := postgres.NewTransactor(pool)
	now := time.Now().UTC()

	errs := runConcurrently(workers, func(i int) error {
		return tx.InTx(context.Background(), func(ctx context.Context, repos ports.Repositories) error {
			return repos.Loans.Insert(ctx, newLoan(item, fmt.Sprintf("sub-%d", i), now))
		})
	})

	ok, rejected := countOutcomes(t, errs, domain.ErrItemAlreadyBorrowed)
	if ok != 1 || rejected != workers-1 {
		t.Fatalf("successes=%d rejected=%d", ok, rejected)
	}
}

func TestRepo_RowLockSerializesBorrowers(t *testing.T) {
	pool := newPool(t)
	item := seedItem(t, pool, "INV-1", domain.ItemAvailable)
	tx := postgres.NewTransactor(pool)
	now := time.Now().UTC()

	errs := runConcurrently(workers, func(i int) error {
		return tx.InTx(context.Background(), func(ctx context.Context, repos ports.Repositories) error {
			got, err := repos.Items.GetForUpdate(ctx, item)
			if err != nil {
				return err
			}
			if got.Status != domain.ItemAvailable {
				return domain.ErrItemAlreadyBorrowed
			}
			if err := repos.Loans.Insert(ctx, newLoan(item, fmt.Sprintf("sub-%d", i), now)); err != nil {
				return err
			}
			return repos.Items.UpdateStatus(ctx, item, domain.ItemBorrowed)
		})
	})

	ok, rejected := countOutcomes(t, errs, domain.ErrItemAlreadyBorrowed)
	if ok != 1 || rejected != workers-1 {
		t.Fatalf("successes=%d rejected=%d", ok, rejected)
	}
}

func TestRepo_LockBorrowerCapsActiveLoans(t *testing.T) {
	pool := newPool(t)
	items := make([]domain.ItemID, workers)
	for i := range items {
		items[i] = seedItem(t, pool, fmt.Sprintf("INV-%d", i), domain.ItemAvailable)
	}
	tx := postgres.NewTransactor(pool)
	now := time.Now().UTC()

	errs := runConcurrently(workers, func(i int) error {
		return tx.InTx(context.Background(), func(ctx context.Context, repos ports.Repositories) error {
			if err := repos.Loans.LockBorrower(ctx, "student-1"); err != nil {
				return err
			}
			active, err := repos.Loans.ListActiveByBorrower(ctx, "student-1")
			if err != nil {
				return err
			}
			if len(active) >= domain.MaxActiveLoansPerBorrower {
				return domain.ErrQuotaExceeded
			}
			return repos.Loans.Insert(ctx, newLoan(items[i], "student-1", now))
		})
	})

	ok, rejected := countOutcomes(t, errs, domain.ErrQuotaExceeded)
	if ok != domain.MaxActiveLoansPerBorrower || rejected != workers-domain.MaxActiveLoansPerBorrower {
		t.Fatalf("successes=%d rejected=%d", ok, rejected)
	}
}
