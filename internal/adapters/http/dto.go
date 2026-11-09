package httpapi

import (
	"time"

	"github.com/ynov/equipment-loans/internal/domain"
)

type modelDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	MaxLoanDays int    `json:"max_loan_days"`
}

type itemDTO struct {
	ID              string   `json:"id"`
	InventoryNumber string   `json:"inventory_number"`
	Status          string   `json:"status"`
	Model           modelDTO `json:"model"`
}

type loanDTO struct {
	ID             string  `json:"id"`
	ItemID         string  `json:"item_id"`
	BorrowerSub    string  `json:"borrower_sub"`
	BorrowedAt     string  `json:"borrowed_at"`
	DueAt          string  `json:"due_at"`
	ReturnedAt     *string `json:"returned_at"`
	ReturnState    *string `json:"return_state"`
	ExtensionsUsed int     `json:"extensions_used"`
}

type borrowRequest struct {
	ItemID      string `json:"item_id"`
	BorrowerSub string `json:"borrower_sub"`
}

type returnRequest struct {
	State string `json:"state"`
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func toItemDTO(v domain.ItemView) itemDTO {
	return itemDTO{
		ID:              string(v.Item.ID),
		InventoryNumber: v.Item.InventoryNumber,
		Status:          string(v.Item.Status),
		Model: modelDTO{
			ID:          string(v.Model.ID),
			Name:        v.Model.Name,
			Category:    v.Model.Category,
			MaxLoanDays: v.Model.MaxLoanDays,
		},
	}
}

func toItemDTOs(views []domain.ItemView) []itemDTO {
	out := make([]itemDTO, 0, len(views))
	for _, v := range views {
		out = append(out, toItemDTO(v))
	}
	return out
}

func toLoanDTO(l domain.Loan) loanDTO {
	dto := loanDTO{
		ID:             string(l.ID),
		ItemID:         string(l.ItemID),
		BorrowerSub:    l.BorrowerSub,
		BorrowedAt:     formatTime(l.BorrowedAt),
		DueAt:          formatTime(l.DueAt),
		ExtensionsUsed: l.ExtensionsUsed,
	}
	if l.ReturnedAt != nil {
		s := formatTime(*l.ReturnedAt)
		dto.ReturnedAt = &s
	}
	if l.ReturnState != nil {
		s := string(*l.ReturnState)
		dto.ReturnState = &s
	}
	return dto
}

func toLoanDTOs(loans []domain.Loan) []loanDTO {
	out := make([]loanDTO, 0, len(loans))
	for _, l := range loans {
		out = append(out, toLoanDTO(l))
	}
	return out
}
