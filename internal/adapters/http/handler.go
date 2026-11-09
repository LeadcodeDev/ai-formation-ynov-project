package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/service"
)

type Service interface {
	ListItems(ctx context.Context, actor domain.Identity, filter domain.ItemFilter) ([]domain.ItemView, error)
	BorrowItem(ctx context.Context, actor domain.Identity, cmd service.BorrowItem) (domain.Loan, error)
	ReturnLoan(ctx context.Context, actor domain.Identity, cmd service.ReturnLoan) (domain.Loan, error)
	ExtendLoan(ctx context.Context, actor domain.Identity, id domain.LoanID) (domain.Loan, error)
	MyLoans(ctx context.Context, actor domain.Identity) ([]domain.Loan, error)
	OverdueLoans(ctx context.Context, actor domain.Identity) ([]domain.Loan, error)
}

type Verifier interface {
	Verify(ctx context.Context, token string) (domain.Identity, error)
}

type handler struct {
	svc    Service
	logger *slog.Logger
}

func NewHandler(svc Service, verifier Verifier, logger *slog.Logger) http.Handler {
	h := &handler{svc: svc, logger: logger}
	protected := func(fn http.HandlerFunc) http.Handler {
		return authenticate(verifier, h, fn)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.Handle("GET /items", protected(h.listItems))
	mux.Handle("POST /loans", protected(h.borrowItem))
	mux.Handle("GET /loans/me", protected(h.myLoans))
	mux.Handle("GET /loans/overdue", protected(h.overdueLoans))
	mux.Handle("POST /loans/{id}/return", protected(h.returnLoan))
	mux.Handle("POST /loans/{id}/extend", protected(h.extendLoan))
	return secured(routed(mux))
}
