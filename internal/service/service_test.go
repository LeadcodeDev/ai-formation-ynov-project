package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/service"
	"github.com/ynov/equipment-loans/internal/service/servicetest"
)

var (
	staff   = domain.Identity{Sub: "staff-1", Roles: []domain.Role{domain.RoleStaff}}
	admin   = domain.Identity{Sub: "admin-1", Roles: []domain.Role{domain.RoleAdmin}}
	alice   = domain.Identity{Sub: "alice", Roles: []domain.Role{domain.RoleStudent}}
	bob     = domain.Identity{Sub: "bob", Roles: []domain.Role{domain.RoleStudent}}
	paris   = mustParis()
	baseNow = time.Date(2025, 1, 10, 10, 0, 0, 0, paris)
)

func mustParis() *time.Location {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		panic(err)
	}
	return loc
}

type env struct {
	svc   *service.Service
	store *servicetest.Store
	clock *servicetest.Clock
}

func newEnv(items ...domain.Item) *env {
	store := servicetest.NewStore()
	store.AddModel(domain.EquipmentModel{ID: "laptop", Name: "Laptop", Category: "portable", MaxLoanDays: 7})
	store.AddModel(domain.EquipmentModel{ID: "kit", Name: "Kit", Category: "kit", MaxLoanDays: 14})
	for _, it := range items {
		store.AddItem(it)
	}
	clock := servicetest.NewClock(baseNow)
	return &env{svc: service.New(store, clock, &servicetest.IDs{}), store: store, clock: clock}
}

func item(id string, model domain.ModelID, status domain.ItemStatus) domain.Item {
	return domain.Item{ID: domain.ItemID(id), ModelID: model, InventoryNumber: id, Status: status}
}

func activeLoan(id, itemID, sub string, due time.Time) domain.Loan {
	return domain.Loan{ID: domain.LoanID(id), ItemID: domain.ItemID(itemID), BorrowerSub: sub, BorrowedAt: baseNow.AddDate(0, 0, -3), DueAt: due}
}

func TestAC3BorrowSetsDueAt(t *testing.T) {
	tests := []struct {
		name  string
		model domain.ModelID
		now   time.Time
		want  time.Time
	}{
		{"laptop 7 days", "laptop", baseNow, time.Date(2025, 1, 18, 0, 0, 0, 0, paris)},
		{"kit 14 days", "kit", baseNow, time.Date(2025, 1, 25, 0, 0, 0, 0, paris)},
		{"last second of day", "laptop", time.Date(2025, 1, 10, 23, 59, 59, 0, paris), time.Date(2025, 1, 18, 0, 0, 0, 0, paris)},
		{"dst spring", "laptop", time.Date(2025, 3, 25, 12, 0, 0, 0, paris), time.Date(2025, 4, 2, 0, 0, 0, 0, paris)},
		{"dst fall", "laptop", time.Date(2025, 10, 22, 12, 0, 0, 0, paris), time.Date(2025, 10, 30, 0, 0, 0, 0, paris)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(item("i1", tc.model, domain.ItemAvailable))
			e.clock.Set(tc.now)
			loan, err := e.svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: "i1", BorrowerSub: "alice"})
			if err != nil {
				t.Fatal(err)
			}
			if !loan.DueAt.Equal(tc.want) {
				t.Fatalf("got %s want %s", loan.DueAt, tc.want)
			}
			if !loan.BorrowedAt.Equal(tc.now) || loan.BorrowerSub != "alice" || loan.ID == "" {
				t.Fatalf("unexpected loan %+v", loan)
			}
			if e.store.Items["i1"].Status != domain.ItemBorrowed {
				t.Fatalf("item status %s", e.store.Items["i1"].Status)
			}
		})
	}
}

func TestAC2BorrowQuota(t *testing.T) {
	due := time.Date(2025, 1, 18, 0, 0, 0, 0, paris)
	tests := []struct {
		name   string
		active int
		want   error
	}{
		{"zero", 0, nil},
		{"two", 2, nil},
		{"three", 3, domain.ErrQuotaExceeded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(item("new", "laptop", domain.ItemAvailable))
			ids := []string{"a", "b", "c"}
			for i := 0; i < tc.active; i++ {
				e.store.AddItem(item("x"+ids[i], "laptop", domain.ItemBorrowed))
				e.store.AddLoan(activeLoan("l"+ids[i], "x"+ids[i], "alice", due))
			}
			_, err := e.svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: "new", BorrowerSub: "alice"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if len(e.store.Locks) == 0 && tc.want != nil {
				t.Fatal("borrower not locked")
			}
		})
	}
}

func TestAC2QuotaIgnoresReturnedAndOthers(t *testing.T) {
	e := newEnv(item("new", "laptop", domain.ItemAvailable))
	due := time.Date(2025, 1, 18, 0, 0, 0, 0, paris)
	returned := baseNow
	for _, id := range []string{"a", "b", "c"} {
		e.store.AddItem(item("x"+id, "laptop", domain.ItemAvailable))
		l := activeLoan("l"+id, "x"+id, "alice", due)
		l.ReturnedAt = &returned
		e.store.AddLoan(l)
	}
	e.store.AddItem(item("xo", "laptop", domain.ItemBorrowed))
	e.store.AddLoan(activeLoan("lo", "xo", "bob", due))
	if _, err := e.svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: "new", BorrowerSub: "alice"}); err != nil {
		t.Fatal(err)
	}
}

func TestAC4OverdueReportedBeforeQuota(t *testing.T) {
	overdueDue := baseNow.Add(-time.Nanosecond)
	futureDue := time.Date(2025, 1, 18, 0, 0, 0, 0, paris)
	tests := []struct {
		name string
		dues []time.Time
		want error
	}{
		{"overdue and quota full", []time.Time{overdueDue, futureDue, futureDue}, domain.ErrBorrowerHasOverdueLoan},
		{"overdue only", []time.Time{overdueDue}, domain.ErrBorrowerHasOverdueLoan},
		{"quota full no overdue", []time.Time{futureDue, futureDue, futureDue}, domain.ErrQuotaExceeded},
		{"due exactly now is not overdue", []time.Time{baseNow}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(item("new", "laptop", domain.ItemAvailable))
			ids := []string{"a", "b", "c"}
			for i, due := range tc.dues {
				e.store.AddItem(item("x"+ids[i], "laptop", domain.ItemBorrowed))
				e.store.AddLoan(activeLoan("l"+ids[i], "x"+ids[i], "alice", due))
			}
			_, err := e.svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: "new", BorrowerSub: "alice"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestAC6BorrowItemStates(t *testing.T) {
	tests := []struct {
		name   string
		status domain.ItemStatus
		want   error
	}{
		{"available", domain.ItemAvailable, nil},
		{"borrowed", domain.ItemBorrowed, domain.ErrItemNotBorrowable},
		{"maintenance", domain.ItemMaintenance, domain.ErrItemNotBorrowable},
		{"lost", domain.ItemLost, domain.ErrItemNotBorrowable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(item("i1", "laptop", tc.status))
			_, err := e.svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: "i1", BorrowerSub: "alice"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestAC6BorrowUnknownItemAndModel(t *testing.T) {
	e := newEnv(domain.Item{ID: "orphan", ModelID: "ghost", Status: domain.ItemAvailable})
	ctx := context.Background()
	if _, err := e.svc.BorrowItem(ctx, staff, service.BorrowItem{ItemID: "nope", BorrowerSub: "alice"}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := e.svc.BorrowItem(ctx, staff, service.BorrowItem{ItemID: "orphan", BorrowerSub: "alice"}); !errors.Is(err, domain.ErrModelNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestAC1BorrowSameItemTwiceSecondFails(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemAvailable))
	ctx := context.Background()
	if _, err := e.svc.BorrowItem(ctx, staff, service.BorrowItem{ItemID: "i1", BorrowerSub: "alice"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.svc.BorrowItem(ctx, staff, service.BorrowItem{ItemID: "i1", BorrowerSub: "bob"})
	if !errors.Is(err, domain.ErrItemNotBorrowable) {
		t.Fatalf("got %v", err)
	}
}

func TestAC8BorrowCheckOrder(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemBorrowed))
	ctx := context.Background()
	if _, err := e.svc.BorrowItem(ctx, alice, service.BorrowItem{ItemID: "missing", BorrowerSub: "alice"}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("forbidden must come first, got %v", err)
	}
	if _, err := e.svc.BorrowItem(ctx, staff, service.BorrowItem{ItemID: "missing", BorrowerSub: ""}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("invalid input must precede lookup, got %v", err)
	}
	if _, err := e.svc.BorrowItem(ctx, staff, service.BorrowItem{ItemID: "missing", BorrowerSub: "alice"}); !errors.Is(err, domain.ErrItemNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestAC4NotBorrowableBeforeOverdue(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemMaintenance), item("x", "laptop", domain.ItemBorrowed))
	e.store.AddLoan(activeLoan("l1", "x", "alice", baseNow.Add(-time.Hour)))
	_, err := e.svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: "i1", BorrowerSub: "alice"})
	if !errors.Is(err, domain.ErrItemNotBorrowable) {
		t.Fatalf("got %v", err)
	}
}

func TestAC8AdminCanBorrow(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemAvailable))
	if _, err := e.svc.BorrowItem(context.Background(), admin, service.BorrowItem{ItemID: "i1", BorrowerSub: "alice"}); err != nil {
		t.Fatal(err)
	}
}

func TestAC5ExtendLoan(t *testing.T) {
	due := time.Date(2025, 1, 18, 0, 0, 0, 0, paris)
	returned := baseNow
	tests := []struct {
		name    string
		mutate  func(*domain.Loan)
		now     time.Time
		actor   domain.Identity
		want    error
		wantDue time.Time
	}{
		{"ok", func(l *domain.Loan) {}, baseNow, alice, nil, time.Date(2025, 1, 25, 0, 0, 0, 0, paris)},
		{"ok at due exactly", func(l *domain.Loan) {}, due, alice, nil, time.Date(2025, 1, 25, 0, 0, 0, 0, paris)},
		{"overdue by 1ns", func(l *domain.Loan) {}, due.Add(time.Nanosecond), alice, domain.ErrLoanOverdue, time.Time{}},
		{"limit reached", func(l *domain.Loan) { l.ExtensionsUsed = 1 }, baseNow, alice, domain.ErrExtensionLimitReached, time.Time{}},
		{"overdue and limit reached", func(l *domain.Loan) { l.ExtensionsUsed = 1 }, due.Add(time.Hour), alice, domain.ErrLoanOverdue, time.Time{}},
		{"already returned", func(l *domain.Loan) { l.ReturnedAt = &returned }, baseNow, alice, domain.ErrLoanAlreadyReturned, time.Time{}},
		{"other borrower", func(l *domain.Loan) {}, baseNow, bob, domain.ErrLoanNotFound, time.Time{}},
		{"staff is not the borrower", func(l *domain.Loan) {}, baseNow, staff, domain.ErrLoanNotFound, time.Time{}},
		{"other borrower returned loan", func(l *domain.Loan) { l.ReturnedAt = &returned }, baseNow, bob, domain.ErrLoanNotFound, time.Time{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(item("i1", "laptop", domain.ItemBorrowed))
			l := activeLoan("l1", "i1", "alice", due)
			tc.mutate(&l)
			e.store.AddLoan(l)
			e.clock.Set(tc.now)
			got, err := e.svc.ExtendLoan(context.Background(), tc.actor, "l1")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if tc.want != nil {
				if e.store.Loans["l1"].ExtensionsUsed != l.ExtensionsUsed {
					t.Fatal("loan mutated on error")
				}
				return
			}
			if !got.DueAt.Equal(tc.wantDue) || got.ExtensionsUsed != 1 {
				t.Fatalf("got %+v", got)
			}
			if !e.store.Loans["l1"].DueAt.Equal(tc.wantDue) {
				t.Fatal("not persisted")
			}
		})
	}
}

func TestAC5ExtendKitAndSecondExtension(t *testing.T) {
	e := newEnv(item("i1", "kit", domain.ItemBorrowed))
	e.store.AddLoan(activeLoan("l1", "i1", "alice", time.Date(2025, 1, 25, 0, 0, 0, 0, paris)))
	ctx := context.Background()
	got, err := e.svc.ExtendLoan(ctx, alice, "l1")
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2025, 2, 8, 0, 0, 0, 0, paris); !got.DueAt.Equal(want) {
		t.Fatalf("got %s want %s", got.DueAt, want)
	}
	if _, err := e.svc.ExtendLoan(ctx, alice, "l1"); !errors.Is(err, domain.ErrExtensionLimitReached) {
		t.Fatalf("got %v", err)
	}
	if _, err := e.svc.ExtendLoan(ctx, alice, "nope"); !errors.Is(err, domain.ErrLoanNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestAC7ReturnLoan(t *testing.T) {
	due := time.Date(2025, 1, 18, 0, 0, 0, 0, paris)
	tests := []struct {
		name  string
		state domain.ReturnState
		want  domain.ItemStatus
	}{
		{"ok", domain.ReturnOK, domain.ItemAvailable},
		{"damaged", domain.ReturnDamaged, domain.ItemMaintenance},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(item("i1", "laptop", domain.ItemBorrowed))
			e.store.AddLoan(activeLoan("l1", "i1", "alice", due))
			ctx := context.Background()
			got, err := e.svc.ReturnLoan(ctx, staff, service.ReturnLoan{LoanID: "l1", State: tc.state})
			if err != nil {
				t.Fatal(err)
			}
			if got.ReturnedAt == nil || !got.ReturnedAt.Equal(baseNow) || got.ReturnState == nil || *got.ReturnState != tc.state {
				t.Fatalf("got %+v", got)
			}
			if e.store.Items["i1"].Status != tc.want {
				t.Fatalf("item status %s want %s", e.store.Items["i1"].Status, tc.want)
			}
			if e.store.Loans["l1"].Active() {
				t.Fatal("loan still active")
			}
			_, err = e.svc.ReturnLoan(ctx, staff, service.ReturnLoan{LoanID: "l1", State: tc.state})
			if !errors.Is(err, domain.ErrLoanAlreadyReturned) {
				t.Fatalf("second return: %v", err)
			}
		})
	}
}

func TestAC7ReturnLoanErrors(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemBorrowed))
	e.store.AddLoan(activeLoan("l1", "i1", "alice", baseNow))
	ctx := context.Background()
	if _, err := e.svc.ReturnLoan(ctx, staff, service.ReturnLoan{LoanID: "nope", State: domain.ReturnOK}); !errors.Is(err, domain.ErrLoanNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := e.svc.ReturnLoan(ctx, staff, service.ReturnLoan{LoanID: "l1", State: "broken"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("got %v", err)
	}
	if !e.store.Loans["l1"].Active() || e.store.Items["i1"].Status != domain.ItemBorrowed {
		t.Fatal("state changed on error")
	}
}

func TestAC8Authorization(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemAvailable))
	ctx := context.Background()
	tests := []struct {
		name string
		call func(actor domain.Identity) error
		who  domain.Identity
		want error
	}{
		{"borrow student", func(a domain.Identity) error {
			_, err := e.svc.BorrowItem(ctx, a, service.BorrowItem{ItemID: "i1", BorrowerSub: "alice"})
			return err
		}, alice, domain.ErrForbidden},
		{"return student", func(a domain.Identity) error {
			_, err := e.svc.ReturnLoan(ctx, a, service.ReturnLoan{LoanID: "l", State: domain.ReturnOK})
			return err
		}, alice, domain.ErrForbidden},
		{"overdue student", func(a domain.Identity) error {
			_, err := e.svc.OverdueLoans(ctx, a)
			return err
		}, alice, domain.ErrForbidden},
		{"overdue staff", func(a domain.Identity) error {
			_, err := e.svc.OverdueLoans(ctx, a)
			return err
		}, staff, nil},
		{"overdue admin", func(a domain.Identity) error {
			_, err := e.svc.OverdueLoans(ctx, a)
			return err
		}, admin, nil},
		{"list items student", func(a domain.Identity) error {
			_, err := e.svc.ListItems(ctx, a, domain.ItemFilter{})
			return err
		}, alice, nil},
		{"my loans student", func(a domain.Identity) error {
			_, err := e.svc.MyLoans(ctx, a)
			return err
		}, alice, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(tc.who); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestAC8MyLoansFiltersOnSub(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemBorrowed), item("i2", "laptop", domain.ItemBorrowed), item("i3", "laptop", domain.ItemBorrowed))
	due := time.Date(2025, 1, 18, 0, 0, 0, 0, paris)
	returned := baseNow
	e.store.AddLoan(activeLoan("l1", "i1", "alice", due))
	done := activeLoan("l2", "i2", "alice", due)
	done.ReturnedAt = &returned
	e.store.AddLoan(done)
	e.store.AddLoan(activeLoan("l3", "i3", "bob", due))
	got, err := e.svc.MyLoans(context.Background(), alice)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "l1" || got[1].ID != "l2" {
		t.Fatalf("got %+v", got)
	}
	for _, l := range got {
		if l.BorrowerSub != "alice" {
			t.Fatalf("leak %+v", l)
		}
	}
}

func TestAC8OverdueLoansFiltersByNow(t *testing.T) {
	e := newEnv()
	returned := baseNow
	e.store.AddLoan(activeLoan("late", "i1", "alice", baseNow.Add(-time.Nanosecond)))
	e.store.AddLoan(activeLoan("equal", "i2", "alice", baseNow))
	e.store.AddLoan(activeLoan("future", "i3", "bob", baseNow.Add(time.Hour)))
	doneLate := activeLoan("returned-late", "i4", "bob", baseNow.Add(-time.Hour))
	doneLate.ReturnedAt = &returned
	e.store.AddLoan(doneLate)
	got, err := e.svc.OverdueLoans(context.Background(), staff)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "late" {
		t.Fatalf("got %+v", got)
	}
}

func TestListItemsFilter(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemAvailable), item("i2", "laptop", domain.ItemBorrowed))
	ctx := context.Background()
	all, err := e.svc.ListItems(ctx, alice, domain.ItemFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("all: %v %v", all, err)
	}
	avail, err := e.svc.ListItems(ctx, alice, domain.ItemFilter{OnlyAvailable: true})
	if err != nil || len(avail) != 1 || avail[0].Item.ID != "i1" || avail[0].Model.ID != "laptop" {
		t.Fatalf("avail: %v %v", avail, err)
	}
}

func TestAC8NoRoleForbiddenOnEveryMethod(t *testing.T) {
	e := newEnv(item("i1", "laptop", domain.ItemAvailable))
	e.store.AddLoan(activeLoan("l1", "i1", "ghost", baseNow.Add(time.Hour)))
	ctx := context.Background()
	roleless := []domain.Identity{
		{Sub: "ghost"},
		{Sub: "ghost", Roles: []domain.Role{}},
		{Sub: "ghost", Roles: []domain.Role{"unknown"}},
	}
	calls := map[string]func(a domain.Identity) error{
		"ListItems": func(a domain.Identity) error {
			_, err := e.svc.ListItems(ctx, a, domain.ItemFilter{})
			return err
		},
		"BorrowItem": func(a domain.Identity) error {
			_, err := e.svc.BorrowItem(ctx, a, service.BorrowItem{ItemID: "i1", BorrowerSub: "alice"})
			return err
		},
		"ReturnLoan": func(a domain.Identity) error {
			_, err := e.svc.ReturnLoan(ctx, a, service.ReturnLoan{LoanID: "l1", State: domain.ReturnOK})
			return err
		},
		"ExtendLoan": func(a domain.Identity) error {
			_, err := e.svc.ExtendLoan(ctx, a, "l1")
			return err
		},
		"MyLoans": func(a domain.Identity) error {
			_, err := e.svc.MyLoans(ctx, a)
			return err
		},
		"OverdueLoans": func(a domain.Identity) error {
			_, err := e.svc.OverdueLoans(ctx, a)
			return err
		},
	}
	for name, call := range calls {
		for i, actor := range roleless {
			t.Run(name+"/"+string(rune('a'+i)), func(t *testing.T) {
				if err := call(actor); !errors.Is(err, domain.ErrForbidden) {
					t.Fatalf("got %v", err)
				}
			})
		}
	}
	if !e.store.Loans["l1"].Active() || e.store.Items["i1"].Status != domain.ItemAvailable {
		t.Fatal("state changed")
	}
}

func TestAC8BorrowerSubLength(t *testing.T) {
	tests := []struct {
		name string
		sub  string
		want error
	}{
		{"empty", "", domain.ErrInvalidInput},
		{"one", "a", nil},
		{"128", strings.Repeat("a", 128), nil},
		{"129", strings.Repeat("a", 129), domain.ErrInvalidInput},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(item("i1", "laptop", domain.ItemAvailable))
			_, err := e.svc.BorrowItem(context.Background(), staff, service.BorrowItem{ItemID: "i1", BorrowerSub: tc.sub})
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}
