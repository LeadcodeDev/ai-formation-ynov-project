package domain

import (
	"fmt"
	"time"
)

type LoanID string

type ReturnState string

const (
	ReturnOK      ReturnState = "ok"
	ReturnDamaged ReturnState = "damaged"
)

func ParseReturnState(s string) (ReturnState, error) {
	switch st := ReturnState(s); st {
	case ReturnOK, ReturnDamaged:
		return st, nil
	}
	return "", fmt.Errorf("%w: return state %q", ErrInvalidInput, s)
}

const (
	MaxActiveLoansPerBorrower = 3
	MaxExtensionsPerLoan      = 1
)

type Loan struct {
	ID             LoanID
	ItemID         ItemID
	BorrowerSub    string
	BorrowedAt     time.Time
	DueAt          time.Time
	ReturnedAt     *time.Time
	ReturnState    *ReturnState
	ExtensionsUsed int
}

func (l Loan) Active() bool {
	return l.ReturnedAt == nil
}
