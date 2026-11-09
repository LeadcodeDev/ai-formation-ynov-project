package service

import "github.com/ynov/equipment-loans/internal/domain"

type BorrowItem struct {
	ItemID      domain.ItemID
	BorrowerSub string
}

type ReturnLoan struct {
	LoanID domain.LoanID
	State  domain.ReturnState
}
