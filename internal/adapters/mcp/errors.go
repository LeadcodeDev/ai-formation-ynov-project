package mcpserver

import (
	"errors"
	"log/slog"

	"github.com/ynov/equipment-loans/internal/domain"
)

var errUnauthenticated = errors.New("authentication required: no valid user identity on this request")

var errInternal = errors.New("internal error: the operation could not be completed, nothing was changed that you can rely on; try again later")

var domainMessages = []struct {
	target error
	msg    string
}{
	{domain.ErrInvalidInput, "invalid input: check the parameters and try again"},
	{domain.ErrForbidden, "forbidden: your role does not allow this action"},
	{domain.ErrModelNotFound, "equipment model not found"},
	{domain.ErrItemNotFound, "item not found: check item_id with list_available_items"},
	{domain.ErrLoanNotFound, "loan not found: check loan_id with my_loans"},
	{domain.ErrItemNotBorrowable, "item not borrowable: it is borrowed, in maintenance or lost"},
	{domain.ErrItemAlreadyBorrowed, "item already borrowed by someone else"},
	{domain.ErrQuotaExceeded, "borrower quota exceeded: 3 active loans maximum"},
	{domain.ErrBorrowerHasOverdueLoan, "borrower has an overdue loan and must return it first"},
	{domain.ErrLoanAlreadyReturned, "loan already returned"},
	{domain.ErrLoanOverdue, "loan is overdue: it cannot be extended, return the item"},
	{domain.ErrExtensionLimitReached, "extension limit reached: a loan can be extended only once"},
}

func toToolError(err error, logger *slog.Logger) error {
	for _, m := range domainMessages {
		if errors.Is(err, m.target) {
			return errors.New(m.msg)
		}
	}
	logger.Error("mcp tool failed", "error", err)
	return errInternal
}
