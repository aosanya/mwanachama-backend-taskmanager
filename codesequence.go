package mwanachamataskmanager

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (m *taskManager) nextCode(ctx context.Context, tx *gorm.DB, entityType, prefix string) (string, error) {
	number, err := m.claimNumber(ctx, tx, entityType, 1)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%d", prefix, number), nil
}

func (m *taskManager) claimNumber(ctx context.Context, tx *gorm.DB, entityType string, first int) (int, error) {
	table := m.store.Table(roleCodeSequence)

	seed, err := m.store.Row(roleCodeSequence, CodeSequence{EntityType: entityType, NextNumber: first})
	if err != nil {
		return 0, fmt.Errorf("nextCode: %w", err)
	}
	if err := tx.WithContext(ctx).Table(table).
		Clauses(clause.OnConflict{DoNothing: true}).Create(seed).Error; err != nil {
		return 0, fmt.Errorf("nextCode: seed the %s counter: %w", entityType, err)
	}
	if err := m.within(tx.WithContext(ctx), roleCodeSequence).
		Where("entity_type = ? AND next_number < ?", entityType, first).
		UpdateColumn("next_number", first).Error; err != nil {
		return 0, fmt.Errorf("nextCode: raise the %s counter: %w", entityType, err)
	}

	res := m.within(tx.WithContext(ctx), roleCodeSequence).Where("entity_type = ?", entityType).
		UpdateColumn("next_number", gorm.Expr("next_number + 1"))
	if res.Error != nil {
		return 0, fmt.Errorf("nextCode: claim a %s number: %w", entityType, res.Error)
	}
	if res.RowsAffected != 1 {
		return 0, fmt.Errorf("nextCode: the %s counter advanced %d rows, want 1", entityType, res.RowsAffected)
	}

	var rows []map[string]any
	if err := m.within(tx.WithContext(ctx), roleCodeSequence).Where("entity_type = ?", entityType).
		Limit(1).Find(&rows).Error; err != nil {
		return 0, fmt.Errorf("nextCode: read the %s counter back: %w", entityType, err)
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("nextCode: the %s counter vanished after being advanced", entityType)
	}
	var advanced CodeSequence
	if err := decode(m.store.Object(roleCodeSequence), rows[0], &advanced); err != nil {
		return 0, fmt.Errorf("nextCode: read the %s counter back: %w", entityType, err)
	}
	return advanced.NextNumber - 1, nil
}
