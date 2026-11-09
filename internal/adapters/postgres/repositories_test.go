package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ynov/equipment-loans/internal/adapters/postgres"
	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/ports"
)

func TestTransactionRollbackDiscardsWrites(t *testing.T) {
	pool := newPool(t)
	item := seedItem(t, pool, "INV-1", domain.ItemAvailable)
	tx := postgres.NewTransactor(pool)
	boom := errors.New("boom")

	err := tx.InTx(context.Background(), func(ctx context.Context, repos ports.Repositories) error {
		if err := repos.Loans.Insert(ctx, newLoan(item, "s", time.Now().UTC())); err != nil {
			return err
		}
		if err := repos.Items.UpdateStatus(ctx, item, domain.ItemBorrowed); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}

	loans, err := postgres.NewLoanRepository(pool).ListActive(context.Background())
	if err != nil || len(loans) != 0 {
		t.Fatalf("loans=%v err=%v", loans, err)
	}
	views, err := postgres.NewItemRepository(pool).List(context.Background(), domain.ItemFilter{OnlyAvailable: true})
	if err != nil || len(views) != 1 {
		t.Fatalf("views=%v err=%v", views, err)
	}
}

func TestTransactionCommitPersistsWrites(t *testing.T) {
	pool := newPool(t)
	item := seedItem(t, pool, "INV-1", domain.ItemAvailable)
	tx := postgres.NewTransactor(pool)

	err := tx.InTx(context.Background(), func(ctx context.Context, repos ports.Repositories) error {
		if err := repos.Loans.Insert(ctx, newLoan(item, "s", time.Now().UTC())); err != nil {
			return err
		}
		return repos.Items.UpdateStatus(ctx, item, domain.ItemBorrowed)
	})
	if err != nil {
		t.Fatalf("InTx: %v", err)
	}
	loans, err := postgres.NewLoanRepository(pool).ListActive(context.Background())
	if err != nil || len(loans) != 1 {
		t.Fatalf("loans=%v err=%v", loans, err)
	}
}

func TestListItemsFiltersAndOrders(t *testing.T) {
	pool := newPool(t)
	seedItem(t, pool, "INV-3", domain.ItemAvailable)
	seedItem(t, pool, "INV-1", domain.ItemAvailable)
	seedItem(t, pool, "INV-2", domain.ItemMaintenance)
	repo := postgres.NewItemRepository(pool)

	all, err := repo.List(context.Background(), domain.ItemFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || all[0].Item.InventoryNumber != "INV-1" || all[2].Item.InventoryNumber != "INV-3" {
		t.Fatalf("all = %+v", all)
	}
	if all[0].Model.Name != "Raspberry Pi 5" || all[0].Model.MaxLoanDays != 7 || all[0].Model.ID != modelID {
		t.Fatalf("model = %+v", all[0].Model)
	}

	available, err := repo.List(context.Background(), domain.ItemFilter{OnlyAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(available) != 2 {
		t.Fatalf("available = %+v", available)
	}
	for _, v := range available {
		if v.Item.Status != domain.ItemAvailable {
			t.Fatalf("status = %s", v.Item.Status)
		}
	}
}

func TestNotFoundMapsToSentinels(t *testing.T) {
	pool := newPool(t)
	items := postgres.NewItemRepository(pool)
	loans := postgres.NewLoanRepository(pool)
	missing := uuid.NewString()
	ctx := context.Background()

	if _, err := items.GetForUpdate(ctx, domain.ItemID(missing)); !errors.Is(err, domain.ErrItemNotFound) {
		t.Fatalf("item: %v", err)
	}
	if _, err := items.GetModel(ctx, domain.ModelID(missing)); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("model: %v", err)
	}
	if _, err := loans.GetForUpdate(ctx, domain.LoanID(missing)); !errors.Is(err, domain.ErrLoanNotFound) {
		t.Fatalf("loan: %v", err)
	}
	if err := items.UpdateStatus(ctx, domain.ItemID(missing), domain.ItemLost); !errors.Is(err, domain.ErrItemNotFound) {
		t.Fatalf("update: %v", err)
	}
	if err := loans.Update(ctx, domain.Loan{ID: domain.LoanID(missing)}); !errors.Is(err, domain.ErrLoanNotFound) {
		t.Fatalf("update loan: %v", err)
	}
}

func TestLoanRoundTripAndListings(t *testing.T) {
	pool := newPool(t)
	repo := postgres.NewLoanRepository(pool)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	first := newLoan(seedItem(t, pool, "INV-1", domain.ItemBorrowed), "alice", base)
	second := newLoan(seedItem(t, pool, "INV-2", domain.ItemBorrowed), "alice", base.Add(time.Hour))
	other := newLoan(seedItem(t, pool, "INV-3", domain.ItemBorrowed), "bob", base)
	for _, l := range []domain.Loan{first, second, other} {
		if err := repo.Insert(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	got, err := repo.GetForUpdate(ctx, first.ID)
	if err != nil || !got.BorrowedAt.Equal(base) || got.BorrowedAt.Location() != time.UTC || got.ReturnedAt != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	returnedAt := base.Add(2 * time.Hour)
	state := domain.ReturnDamaged
	first.ReturnedAt = &returnedAt
	first.ReturnState = &state
	first.ExtensionsUsed = 1
	if err := repo.Update(ctx, first); err != nil {
		t.Fatal(err)
	}
	got, err = repo.GetForUpdate(ctx, first.ID)
	if err != nil || got.ReturnedAt == nil || !got.ReturnedAt.Equal(returnedAt) || *got.ReturnState != domain.ReturnDamaged || got.ExtensionsUsed != 1 {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	all, err := repo.ListByBorrower(ctx, "alice")
	if err != nil || len(all) != 2 || all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatalf("all=%+v err=%v", all, err)
	}
	active, err := repo.ListActiveByBorrower(ctx, "alice")
	if err != nil || len(active) != 1 || active[0].ID != second.ID {
		t.Fatalf("active=%+v err=%v", active, err)
	}
	everyone, err := repo.ListActive(ctx)
	if err != nil || len(everyone) != 2 {
		t.Fatalf("everyone=%+v err=%v", everyone, err)
	}
}
