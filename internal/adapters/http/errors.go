package httpapi

import (
	"errors"
	"net/http"

	"github.com/ynov/equipment-loans/internal/domain"
)

type errorRule struct {
	err     error
	status  int
	code    string
	message string
}

var errorRules = []errorRule{
	{domain.ErrInvalidInput, http.StatusBadRequest, "invalid_input", "invalid input"},
	{domain.ErrForbidden, http.StatusForbidden, "forbidden", "forbidden"},
	{domain.ErrModelNotFound, http.StatusNotFound, "model_not_found", "equipment model not found"},
	{domain.ErrItemNotFound, http.StatusNotFound, "item_not_found", "item not found"},
	{domain.ErrLoanNotFound, http.StatusNotFound, "loan_not_found", "loan not found"},
	{domain.ErrItemNotBorrowable, http.StatusConflict, "item_not_borrowable", "item not borrowable"},
	{domain.ErrItemAlreadyBorrowed, http.StatusConflict, "item_already_borrowed", "item already borrowed"},
	{domain.ErrQuotaExceeded, http.StatusConflict, "quota_exceeded", "borrower quota exceeded"},
	{domain.ErrBorrowerHasOverdueLoan, http.StatusConflict, "borrower_has_overdue_loan", "borrower has an overdue loan"},
	{domain.ErrLoanAlreadyReturned, http.StatusConflict, "loan_already_returned", "loan already returned"},
	{domain.ErrLoanOverdue, http.StatusConflict, "loan_overdue", "loan overdue"},
	{domain.ErrExtensionLimitReached, http.StatusConflict, "extension_limit_reached", "extension limit reached"},
}

func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		h.logger.WarnContext(r.Context(), "request rejected", "error", err, "method", r.Method, "path", r.URL.Path)
		writeError(w, http.StatusRequestEntityTooLarge, "invalid_input", "request body too large")
		return
	}
	for _, rule := range errorRules {
		if errors.Is(err, rule.err) {
			h.logger.InfoContext(r.Context(), "request rejected", "error", err, "code", rule.code, "method", r.Method, "path", r.URL.Path)
			writeError(w, rule.status, rule.code, rule.message)
			return
		}
	}
	h.logger.ErrorContext(r.Context(), "internal error", "error", err, "method", r.Method, "path", r.URL.Path)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Code: code, Message: message})
}
