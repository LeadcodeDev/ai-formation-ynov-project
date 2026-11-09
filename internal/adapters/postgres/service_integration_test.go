package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ynov/equipment-loans/internal/adapters/postgres"
	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/service"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type uuidGenerator struct{}

func (uuidGenerator) NewID() string { return uuid.NewString() }

func TestAC1_ServiceBorrowSameItemConcurrently(t *testing.T) {
	pool := newPool(t)
	item := seedItem(t, pool, "INV-1", domain.ItemAvailable)
	svc := service.New(postgres.NewTransactor(pool), fixedClock{time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)}, uuidGenerator{})
	staff := domain.Identity{Sub: "staff-1", Roles: []domain.Role{domain.RoleStaff}}

	errs := runConcurrently(workers, func(i int) error {
		_, err := svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: item, BorrowerSub: fmt.Sprintf("student-%d", i)})
		return err
	})

	successes := 0
	for _, err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrItemNotBorrowable), errors.Is(err, domain.ErrItemAlreadyBorrowed):
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d", successes)
	}
}

func TestAC2_ServiceBorrowQuotaConcurrently(t *testing.T) {
	pool := newPool(t)
	items := make([]domain.ItemID, workers)
	for i := range items {
		items[i] = seedItem(t, pool, fmt.Sprintf("INV-%d", i), domain.ItemAvailable)
	}
	svc := service.New(postgres.NewTransactor(pool), fixedClock{time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)}, uuidGenerator{})
	staff := domain.Identity{Sub: "staff-1", Roles: []domain.Role{domain.RoleStaff}}

	errs := runConcurrently(workers, func(i int) error {
		_, err := svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: items[i], BorrowerSub: "student-1"})
		return err
	})

	ok, rejected := countOutcomes(t, errs, domain.ErrQuotaExceeded)
	if ok != domain.MaxActiveLoansPerBorrower || rejected != workers-domain.MaxActiveLoansPerBorrower {
		t.Fatalf("successes=%d rejected=%d", ok, rejected)
	}
}
