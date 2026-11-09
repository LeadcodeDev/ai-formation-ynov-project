package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/ynov/equipment-loans/internal/domain"
)

const loanColumns = "id, item_id, borrower_sub, borrowed_at, due_at, returned_at, return_state, extensions_used"

type LoanRepository struct {
	db db
}

func (r *LoanRepository) Insert(ctx context.Context, loan domain.Loan) error {
	_, err := r.db.Exec(ctx,
		"INSERT INTO loan ("+loanColumns+") VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		string(loan.ID), string(loan.ItemID), loan.BorrowerSub, loan.BorrowedAt, loan.DueAt,
		loan.ReturnedAt, returnStateArg(loan.ReturnState), loan.ExtensionsUsed)
	if isActiveLoanViolation(err) {
		return domain.ErrItemAlreadyBorrowed
	}
	if err != nil {
		return fmt.Errorf("postgres: insert loan: %w", err)
	}
	return nil
}

func returnStateArg(state *domain.ReturnState) *string {
	if state == nil {
		return nil
	}
	s := string(*state)
	return &s
}

func (r *LoanRepository) GetForUpdate(ctx context.Context, id domain.LoanID) (domain.Loan, error) {
	row := r.db.QueryRow(ctx, "SELECT "+loanColumns+" FROM loan WHERE id = $1 FOR UPDATE", string(id))
	loan, err := scanLoan(row)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return domain.Loan{}, domain.ErrLoanNotFound
	}
	if err != nil {
		return domain.Loan{}, fmt.Errorf("postgres: get loan: %w", err)
	}
	return loan, nil
}

func (r *LoanRepository) Update(ctx context.Context, loan domain.Loan) error {
	tag, err := r.db.Exec(ctx,
		"UPDATE loan SET due_at = $2, returned_at = $3, return_state = $4, extensions_used = $5 WHERE id = $1",
		string(loan.ID), loan.DueAt, loan.ReturnedAt, returnStateArg(loan.ReturnState), loan.ExtensionsUsed)
	if isInvalidUUID(err) {
		return domain.ErrLoanNotFound
	}
	if err != nil {
		return fmt.Errorf("postgres: update loan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrLoanNotFound
	}
	return nil
}

func (r *LoanRepository) LockBorrower(ctx context.Context, sub string) error {
	if _, err := r.db.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", sub); err != nil {
		return fmt.Errorf("postgres: lock borrower: %w", err)
	}
	return nil
}

func (r *LoanRepository) ListActiveByBorrower(ctx context.Context, sub string) ([]domain.Loan, error) {
	return r.list(ctx, "WHERE borrower_sub = $1 AND returned_at IS NULL ORDER BY borrowed_at DESC", sub)
}

func (r *LoanRepository) ListByBorrower(ctx context.Context, sub string) ([]domain.Loan, error) {
	return r.list(ctx, "WHERE borrower_sub = $1 ORDER BY borrowed_at DESC", sub)
}

func (r *LoanRepository) ListActive(ctx context.Context) ([]domain.Loan, error) {
	return r.list(ctx, "WHERE returned_at IS NULL ORDER BY due_at")
}

func (r *LoanRepository) list(ctx context.Context, clause string, args ...any) ([]domain.Loan, error) {
	rows, err := r.db.Query(ctx, "SELECT "+loanColumns+" FROM loan "+clause, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list loans: %w", err)
	}
	defer rows.Close()
	loans := []domain.Loan{}
	for rows.Next() {
		loan, err := scanLoan(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: list loans: %w", err)
		}
		loans = append(loans, loan)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list loans: %w", err)
	}
	return loans, nil
}

func scanLoan(row pgx.Row) (domain.Loan, error) {
	var l domain.Loan
	var id, itemID string
	var returnedAt *time.Time
	var state *string
	err := row.Scan(&id, &itemID, &l.BorrowerSub, &l.BorrowedAt, &l.DueAt, &returnedAt, &state, &l.ExtensionsUsed)
	if err != nil {
		return domain.Loan{}, err
	}
	l.ID = domain.LoanID(id)
	l.ItemID = domain.ItemID(itemID)
	l.BorrowedAt = l.BorrowedAt.UTC()
	l.DueAt = l.DueAt.UTC()
	if returnedAt != nil {
		utc := returnedAt.UTC()
		l.ReturnedAt = &utc
	}
	if state != nil {
		rs := domain.ReturnState(*state)
		l.ReturnState = &rs
	}
	return l, nil
}
