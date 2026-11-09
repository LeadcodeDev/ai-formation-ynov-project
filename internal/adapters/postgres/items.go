package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ynov/equipment-loans/internal/domain"
)

type ItemRepository struct {
	db db
}

func (r *ItemRepository) List(ctx context.Context, filter domain.ItemFilter) ([]domain.ItemView, error) {
	query := `SELECT i.id, i.model_id, i.inventory_number, i.status, m.name, m.category, m.max_loan_days
FROM item i JOIN equipment_model m ON m.id = i.model_id
WHERE ($1 = FALSE OR i.status = 'available')
ORDER BY i.inventory_number`
	rows, err := r.db.Query(ctx, query, filter.OnlyAvailable)
	if err != nil {
		return nil, fmt.Errorf("postgres: list items: %w", err)
	}
	defer rows.Close()
	views := []domain.ItemView{}
	for rows.Next() {
		view, err := scanItemView(rows)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list items: %w", err)
	}
	return views, nil
}

func scanItemView(row pgx.Row) (domain.ItemView, error) {
	var v domain.ItemView
	var id, modelID, status string
	err := row.Scan(&id, &modelID, &v.Item.InventoryNumber, &status, &v.Model.Name, &v.Model.Category, &v.Model.MaxLoanDays)
	if err != nil {
		return domain.ItemView{}, fmt.Errorf("postgres: scan item: %w", err)
	}
	v.Item.ID = domain.ItemID(id)
	v.Item.ModelID = domain.ModelID(modelID)
	v.Item.Status = domain.ItemStatus(status)
	v.Model.ID = v.Item.ModelID
	return v, nil
}

func (r *ItemRepository) GetForUpdate(ctx context.Context, id domain.ItemID) (domain.Item, error) {
	var item domain.Item
	var rawID, modelID, status string
	err := r.db.QueryRow(ctx, "SELECT id, model_id, inventory_number, status FROM item WHERE id = $1 FOR UPDATE", string(id)).
		Scan(&rawID, &modelID, &item.InventoryNumber, &status)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return domain.Item{}, domain.ErrItemNotFound
	}
	if err != nil {
		return domain.Item{}, fmt.Errorf("postgres: get item: %w", err)
	}
	item.ID = domain.ItemID(rawID)
	item.ModelID = domain.ModelID(modelID)
	item.Status = domain.ItemStatus(status)
	return item, nil
}

func (r *ItemRepository) UpdateStatus(ctx context.Context, id domain.ItemID, status domain.ItemStatus) error {
	tag, err := r.db.Exec(ctx, "UPDATE item SET status = $2 WHERE id = $1", string(id), string(status))
	if isInvalidUUID(err) {
		return domain.ErrItemNotFound
	}
	if err != nil {
		return fmt.Errorf("postgres: update item status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrItemNotFound
	}
	return nil
}

func (r *ItemRepository) GetModel(ctx context.Context, id domain.ModelID) (domain.EquipmentModel, error) {
	var m domain.EquipmentModel
	var rawID string
	err := r.db.QueryRow(ctx, "SELECT id, name, category, max_loan_days FROM equipment_model WHERE id = $1", string(id)).
		Scan(&rawID, &m.Name, &m.Category, &m.MaxLoanDays)
	if errors.Is(err, pgx.ErrNoRows) || isInvalidUUID(err) {
		return domain.EquipmentModel{}, domain.ErrModelNotFound
	}
	if err != nil {
		return domain.EquipmentModel{}, fmt.Errorf("postgres: get model: %w", err)
	}
	m.ID = domain.ModelID(rawID)
	return m, nil
}
