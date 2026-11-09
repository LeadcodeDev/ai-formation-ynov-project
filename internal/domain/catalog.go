package domain

import "fmt"

type ModelID string

type ItemID string

type ItemStatus string

const (
	ItemAvailable   ItemStatus = "available"
	ItemBorrowed    ItemStatus = "borrowed"
	ItemMaintenance ItemStatus = "maintenance"
	ItemLost        ItemStatus = "lost"
)

func ParseItemStatus(s string) (ItemStatus, error) {
	switch st := ItemStatus(s); st {
	case ItemAvailable, ItemBorrowed, ItemMaintenance, ItemLost:
		return st, nil
	}
	return "", fmt.Errorf("%w: item status %q", ErrInvalidInput, s)
}

type EquipmentModel struct {
	ID          ModelID
	Name        string
	Category    string
	MaxLoanDays int
}

type Item struct {
	ID              ItemID
	ModelID         ModelID
	InventoryNumber string
	Status          ItemStatus
}

type ItemView struct {
	Item  Item
	Model EquipmentModel
}

type ItemFilter struct {
	OnlyAvailable bool
}
