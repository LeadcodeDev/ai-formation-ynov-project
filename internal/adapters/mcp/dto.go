package mcpserver

import (
	"time"

	"github.com/ynov/equipment-loans/internal/domain"
)

type emptyInput struct{}

type borrowInput struct {
	ItemID      string `json:"item_id"`
	BorrowerSub string `json:"borrower_sub"`
}

type returnInput struct {
	LoanID string `json:"loan_id"`
	State  string `json:"state"`
}

type loanInput struct {
	LoanID string `json:"loan_id"`
}

type itemOutput struct {
	ItemID          string `json:"item_id" jsonschema:"item identifier, to pass as item_id to borrow_item"`
	InventoryNumber string `json:"inventory_number" jsonschema:"physical inventory number"`
	ModelName       string `json:"model_name" jsonschema:"equipment model name"`
	Category        string `json:"category" jsonschema:"equipment category"`
	MaxLoanDays     int    `json:"max_loan_days" jsonschema:"loan duration in days for this model"`
}

type itemsOutput struct {
	Items []itemOutput `json:"items" jsonschema:"available items"`
}

type loanOutput struct {
	LoanID         string     `json:"loan_id" jsonschema:"loan identifier"`
	ItemID         string     `json:"item_id" jsonschema:"borrowed item"`
	BorrowerSub    string     `json:"borrower_sub" jsonschema:"borrower subject"`
	BorrowedAt     time.Time  `json:"borrowed_at" jsonschema:"borrow time"`
	DueAt          time.Time  `json:"due_at" jsonschema:"due time, Europe/Paris midnight"`
	ReturnedAt     *time.Time `json:"returned_at,omitempty" jsonschema:"return time when returned"`
	ReturnState    string     `json:"return_state,omitempty" jsonschema:"ok or damaged when returned"`
	ExtensionsUsed int        `json:"extensions_used" jsonschema:"number of extensions used"`
}

type loansOutput struct {
	Loans []loanOutput `json:"loans" jsonschema:"loans"`
}

func toItemsOutput(views []domain.ItemView) itemsOutput {
	items := make([]itemOutput, 0, len(views))
	for _, v := range views {
		items = append(items, itemOutput{
			ItemID:          string(v.Item.ID),
			InventoryNumber: v.Item.InventoryNumber,
			ModelName:       v.Model.Name,
			Category:        v.Model.Category,
			MaxLoanDays:     v.Model.MaxLoanDays,
		})
	}
	return itemsOutput{Items: items}
}

func toLoanOutput(l domain.Loan) loanOutput {
	out := loanOutput{
		LoanID:         string(l.ID),
		ItemID:         string(l.ItemID),
		BorrowerSub:    l.BorrowerSub,
		BorrowedAt:     l.BorrowedAt,
		DueAt:          l.DueAt,
		ReturnedAt:     l.ReturnedAt,
		ExtensionsUsed: l.ExtensionsUsed,
	}
	if l.ReturnState != nil {
		out.ReturnState = string(*l.ReturnState)
	}
	return out
}

func toLoansOutput(loans []domain.Loan) loansOutput {
	out := make([]loanOutput, 0, len(loans))
	for _, l := range loans {
		out = append(out, toLoanOutput(l))
	}
	return loansOutput{Loans: out}
}
