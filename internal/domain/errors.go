package domain

import "errors"

var (
	ErrInvalidInput           = errors.New("invalid input")
	ErrForbidden              = errors.New("forbidden")
	ErrModelNotFound          = errors.New("equipment model not found")
	ErrItemNotFound           = errors.New("item not found")
	ErrLoanNotFound           = errors.New("loan not found")
	ErrItemNotBorrowable      = errors.New("item not borrowable")
	ErrItemAlreadyBorrowed    = errors.New("item already borrowed")
	ErrQuotaExceeded          = errors.New("borrower quota exceeded")
	ErrBorrowerHasOverdueLoan = errors.New("borrower has an overdue loan")
	ErrLoanAlreadyReturned    = errors.New("loan already returned")
	ErrLoanOverdue            = errors.New("loan overdue")
	ErrExtensionLimitReached  = errors.New("extension limit reached")
)
