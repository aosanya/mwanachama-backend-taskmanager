package gormstore

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// CodeSequenceRow tracks the next number to mint for one entity type's
// stable business Code (e.g. "D-1", "D-2", ...). One row per entity type.
type CodeSequenceRow struct {
	EntityType string `gorm:"primaryKey"`
	NextNumber int
}

// NextCode reads-or-creates entityType's counter row within tx, increments
// it, and returns "<prefix>-<n>". Callers must run this inside the same
// transaction as the row Create it mints a code for, so the counter
// increment and the insert commit atomically.
func NextCode(ctx context.Context, tx *gorm.DB, sequenceTable, entityType, prefix string) (string, error) {
	var seq CodeSequenceRow
	err := tx.WithContext(ctx).Table(sequenceTable).Where("entity_type = ?", entityType).First(&seq).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		seq = CodeSequenceRow{EntityType: entityType, NextNumber: 1}
		if err := tx.WithContext(ctx).Table(sequenceTable).Create(&seq).Error; err != nil {
			return "", fmt.Errorf("NextCode: %w", err)
		}
	case err != nil:
		return "", fmt.Errorf("NextCode: %w", err)
	}
	n := seq.NextNumber
	if err := tx.WithContext(ctx).Table(sequenceTable).Where("entity_type = ?", entityType).
		Update("next_number", n+1).Error; err != nil {
		return "", fmt.Errorf("NextCode: %w", err)
	}
	return fmt.Sprintf("%s-%d", prefix, n), nil
}

// BackfillCodes assigns a Code to every row in table with an empty code,
// ordered by created_at, so pre-existing rows (from before Code existed)
// get one without disturbing already-coded rows. Idempotent — safe to call
// on every Migrate.
func BackfillCodes(db *gorm.DB, table, sequenceTable, entityType, prefix string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Table(table).Where("code = '' OR code IS NULL").Order("created_at").Pluck("id", &ids).Error; err != nil {
			return fmt.Errorf("BackfillCodes: %w", err)
		}
		for _, id := range ids {
			code, err := NextCode(context.Background(), tx, sequenceTable, entityType, prefix)
			if err != nil {
				return err
			}
			if err := tx.Table(table).Where("id = ?", id).Update("code", code).Error; err != nil {
				return fmt.Errorf("BackfillCodes: %w", err)
			}
		}
		return nil
	})
}
