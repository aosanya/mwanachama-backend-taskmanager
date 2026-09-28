package mwanachamataskmanager

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

func (m *taskManager) nextCode(ctx context.Context, tx *gorm.DB, entityType, prefix string) (string, error) {
	table := m.store.Table(roleCodeSequence)
	object := m.store.Object(roleCodeSequence)

	var rows []map[string]any
	if err := tx.WithContext(ctx).Table(table).Where("entity_type = ?", entityType).
		Limit(1).Find(&rows).Error; err != nil {
		return "", fmt.Errorf("nextCode: %w", err)
	}

	current := CodeSequence{EntityType: entityType, NextNumber: 1}
	if len(rows) == 0 {
		row, err := encode(object, current)
		if err != nil {
			return "", fmt.Errorf("nextCode: %w", err)
		}
		if err := tx.WithContext(ctx).Table(table).Create(row).Error; err != nil {
			return "", fmt.Errorf("nextCode: %w", err)
		}
	} else if err := decode(object, rows[0], &current); err != nil {
		return "", fmt.Errorf("nextCode: %w", err)
	}

	n := current.NextNumber
	if err := tx.WithContext(ctx).Table(table).Where("entity_type = ?", entityType).
		Update("next_number", n+1).Error; err != nil {
		return "", fmt.Errorf("nextCode: %w", err)
	}
	return fmt.Sprintf("%s-%d", prefix, n), nil
}
