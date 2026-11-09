package ports

import (
	"context"
	"time"

	"github.com/ynov/equipment-loans/internal/domain"
)

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	NewID() string
}

type ItemRepository interface {
	List(ctx context.Context, filter domain.ItemFilter) ([]domain.ItemView, error)
	GetForUpdate(ctx context.Context, id domain.ItemID) (domain.Item, error)
	UpdateStatus(ctx context.Context, id domain.ItemID, status domain.ItemStatus) error
	GetModel(ctx context.Context, id domain.ModelID) (domain.EquipmentModel, error)
}

type LoanRepository interface {
	Insert(ctx context.Context, loan domain.Loan) error
	GetForUpdate(ctx context.Context, id domain.LoanID) (domain.Loan, error)
	Update(ctx context.Context, loan domain.Loan) error
	LockBorrower(ctx context.Context, sub string) error
	ListActiveByBorrower(ctx context.Context, sub string) ([]domain.Loan, error)
	ListByBorrower(ctx context.Context, sub string) ([]domain.Loan, error)
	ListActive(ctx context.Context) ([]domain.Loan, error)
}

type Repositories struct {
	Items ItemRepository
	Loans LoanRepository
}

type Transactor interface {
	InTx(ctx context.Context, fn func(ctx context.Context, repos Repositories) error) error
}
