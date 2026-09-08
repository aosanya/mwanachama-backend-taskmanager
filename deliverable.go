package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// ListDeliverablesForTask returns all Deliverable rows linked to taskID via
// has_deliverable (i.e. whose ParentID equals taskID).
func (m *taskManager) ListDeliverablesForTask(ctx context.Context, taskID string) ([]Deliverable, error) {
	var rows []gormstore.DeliverableRow
	if err := m.db.WithContext(ctx).Table(m.tables.Deliverables).Where("parent_id = ?", taskID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListDeliverablesForTask: %w", err)
	}
	out := make([]Deliverable, 0, len(rows))
	for _, r := range rows {
		out = append(out, gormstore.DeliverableFromRow(r))
	}
	return out, nil
}

// ListAcceptanceCriteriaForTask returns all AcceptanceCriteria rows linked
// to taskID via has_acceptance_criteria (i.e. whose ParentID equals taskID).
func (m *taskManager) ListAcceptanceCriteriaForTask(ctx context.Context, taskID string) ([]AcceptanceCriteria, error) {
	var rows []gormstore.AcceptanceCriteriaRow
	if err := m.db.WithContext(ctx).Table(m.tables.AcceptanceCriteria).Where("parent_id = ?", taskID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListAcceptanceCriteriaForTask: %w", err)
	}
	out := make([]AcceptanceCriteria, 0, len(rows))
	for _, r := range rows {
		out = append(out, gormstore.AcceptanceCriteriaFromRow(r))
	}
	return out, nil
}

// WriteAcceptanceCriteriaResult writes the reviewer's result and
// result_notes onto an AcceptanceCriteria row.
func (m *taskManager) WriteAcceptanceCriteriaResult(ctx context.Context, criteriaID, result, notes string) error {
	res := m.db.WithContext(ctx).Table(m.tables.AcceptanceCriteria).Where("id = ?", criteriaID).
		Updates(map[string]any{
			"result":       result,
			"result_notes": notes,
			"updated_at":   time.Now().UTC().Format(time.RFC3339),
		})
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return ErrAcceptanceCriteriaNotFound
		}
		return fmt.Errorf("WriteAcceptanceCriteriaResult: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrAcceptanceCriteriaNotFound
	}
	return nil
}
