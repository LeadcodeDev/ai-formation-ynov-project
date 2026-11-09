package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/ports"
)

type Service struct {
	tx    ports.Transactor
	clock ports.Clock
	ids   ports.IDGenerator
}

func New(tx ports.Transactor, clock ports.Clock, ids ports.IDGenerator) *Service {
	return &Service{tx: tx, clock: clock, ids: ids}
}

const maxBorrowerSubLength = 128

func requireRole(actor domain.Identity, min domain.Role) error {
	if min == domain.RoleStaff && actor.IsStaff() {
		return nil
	}
	if min == domain.RoleStudent && (actor.Has(domain.RoleStudent) || actor.IsStaff()) {
		return nil
	}
	return domain.ErrForbidden
}

func validateBorrowerSub(sub string) error {
	if sub == "" || len(sub) > maxBorrowerSubLength {
		return fmt.Errorf("%w: borrower must be 1 to %d characters", domain.ErrInvalidInput, maxBorrowerSubLength)
	}
	return nil
}

func (s *Service) ListItems(ctx context.Context, actor domain.Identity, filter domain.ItemFilter) ([]domain.ItemView, error) {
	if err := requireRole(actor, domain.RoleStudent); err != nil {
		return nil, err
	}
	var out []domain.ItemView
	err := s.tx.InTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		var err error
		out, err = repos.Items.List(ctx, filter)
		return err
	})
	return out, err
}

func (s *Service) BorrowItem(ctx context.Context, actor domain.Identity, cmd BorrowItem) (domain.Loan, error) {
	if err := requireRole(actor, domain.RoleStaff); err != nil {
		return domain.Loan{}, err
	}
	if err := validateBorrowerSub(cmd.BorrowerSub); err != nil {
		return domain.Loan{}, err
	}
	var loan domain.Loan
	err := s.tx.InTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		var err error
		loan, err = s.borrow(ctx, repos, cmd)
		return err
	})
	return loan, err
}

func (s *Service) borrow(ctx context.Context, repos ports.Repositories, cmd BorrowItem) (domain.Loan, error) {
	item, err := repos.Items.GetForUpdate(ctx, cmd.ItemID)
	if err != nil {
		return domain.Loan{}, err
	}
	if item.Status != domain.ItemAvailable {
		return domain.Loan{}, fmt.Errorf("%w: status %s", domain.ErrItemNotBorrowable, item.Status)
	}
	model, err := repos.Items.GetModel(ctx, item.ModelID)
	if err != nil {
		return domain.Loan{}, err
	}
	now := s.clock.Now()
	if err := s.checkBorrower(ctx, repos, cmd.BorrowerSub, now); err != nil {
		return domain.Loan{}, err
	}
	loan := domain.Loan{
		ID:          domain.LoanID(s.ids.NewID()),
		ItemID:      item.ID,
		BorrowerSub: cmd.BorrowerSub,
		BorrowedAt:  now,
		DueAt:       domain.DueAt(now, model.MaxLoanDays),
	}
	if err := repos.Loans.Insert(ctx, loan); err != nil {
		return domain.Loan{}, err
	}
	if err := repos.Items.UpdateStatus(ctx, item.ID, domain.ItemBorrowed); err != nil {
		return domain.Loan{}, err
	}
	return loan, nil
}

func (s *Service) checkBorrower(ctx context.Context, repos ports.Repositories, sub string, now time.Time) error {
	if err := repos.Loans.LockBorrower(ctx, sub); err != nil {
		return err
	}
	active, err := repos.Loans.ListActiveByBorrower(ctx, sub)
	if err != nil {
		return err
	}
	for _, l := range active {
		if domain.IsOverdue(l, now) {
			return domain.ErrBorrowerHasOverdueLoan
		}
	}
	if len(active) >= domain.MaxActiveLoansPerBorrower {
		return domain.ErrQuotaExceeded
	}
	return nil
}

func (s *Service) ReturnLoan(ctx context.Context, actor domain.Identity, cmd ReturnLoan) (domain.Loan, error) {
	if err := requireRole(actor, domain.RoleStaff); err != nil {
		return domain.Loan{}, err
	}
	state, err := domain.ParseReturnState(string(cmd.State))
	if err != nil {
		return domain.Loan{}, err
	}
	var loan domain.Loan
	err = s.tx.InTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		var err error
		loan, err = s.giveBack(ctx, repos, cmd.LoanID, state)
		return err
	})
	return loan, err
}

func (s *Service) giveBack(ctx context.Context, repos ports.Repositories, id domain.LoanID, state domain.ReturnState) (domain.Loan, error) {
	loan, err := repos.Loans.GetForUpdate(ctx, id)
	if err != nil {
		return domain.Loan{}, err
	}
	if !loan.Active() {
		return domain.Loan{}, domain.ErrLoanAlreadyReturned
	}
	now := s.clock.Now()
	loan.ReturnedAt = &now
	loan.ReturnState = &state
	if err := repos.Loans.Update(ctx, loan); err != nil {
		return domain.Loan{}, err
	}
	if err := repos.Items.UpdateStatus(ctx, loan.ItemID, statusAfterReturn(state)); err != nil {
		return domain.Loan{}, err
	}
	return loan, nil
}

func statusAfterReturn(state domain.ReturnState) domain.ItemStatus {
	if state == domain.ReturnDamaged {
		return domain.ItemMaintenance
	}
	return domain.ItemAvailable
}

func (s *Service) ExtendLoan(ctx context.Context, actor domain.Identity, id domain.LoanID) (domain.Loan, error) {
	if err := requireRole(actor, domain.RoleStudent); err != nil {
		return domain.Loan{}, err
	}
	var loan domain.Loan
	err := s.tx.InTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		var err error
		loan, err = s.extend(ctx, repos, actor, id)
		return err
	})
	return loan, err
}

func (s *Service) extend(ctx context.Context, repos ports.Repositories, actor domain.Identity, id domain.LoanID) (domain.Loan, error) {
	loan, err := repos.Loans.GetForUpdate(ctx, id)
	if err != nil {
		return domain.Loan{}, err
	}
	if loan.BorrowerSub != actor.Sub {
		return domain.Loan{}, domain.ErrLoanNotFound
	}
	if !loan.Active() {
		return domain.Loan{}, domain.ErrLoanAlreadyReturned
	}
	if domain.IsOverdue(loan, s.clock.Now()) {
		return domain.Loan{}, domain.ErrLoanOverdue
	}
	if loan.ExtensionsUsed >= domain.MaxExtensionsPerLoan {
		return domain.Loan{}, domain.ErrExtensionLimitReached
	}
	item, err := repos.Items.GetForUpdate(ctx, loan.ItemID)
	if err != nil {
		return domain.Loan{}, err
	}
	model, err := repos.Items.GetModel(ctx, item.ModelID)
	if err != nil {
		return domain.Loan{}, err
	}
	loan.DueAt = domain.ExtendedDueAt(loan.DueAt, model.MaxLoanDays)
	loan.ExtensionsUsed++
	if err := repos.Loans.Update(ctx, loan); err != nil {
		return domain.Loan{}, err
	}
	return loan, nil
}

func (s *Service) MyLoans(ctx context.Context, actor domain.Identity) ([]domain.Loan, error) {
	if err := requireRole(actor, domain.RoleStudent); err != nil {
		return nil, err
	}
	var out []domain.Loan
	err := s.tx.InTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		var err error
		out, err = repos.Loans.ListByBorrower(ctx, actor.Sub)
		return err
	})
	return out, err
}

func (s *Service) OverdueLoans(ctx context.Context, actor domain.Identity) ([]domain.Loan, error) {
	if err := requireRole(actor, domain.RoleStaff); err != nil {
		return nil, err
	}
	var out []domain.Loan
	err := s.tx.InTx(ctx, func(ctx context.Context, repos ports.Repositories) error {
		active, err := repos.Loans.ListActive(ctx)
		if err != nil {
			return err
		}
		now := s.clock.Now()
		for _, l := range active {
			if domain.IsOverdue(l, now) {
				out = append(out, l)
			}
		}
		return nil
	})
	return out, err
}
