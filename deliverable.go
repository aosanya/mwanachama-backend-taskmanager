package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

// ListDeliverablesForTask returns all Deliverable rows linked to taskID via
// has_deliverable (i.e. whose ParentID equals taskID).
func (m *taskManager) ListDeliverablesForTask(ctx context.Context, taskID string) ([]Deliverable, error) {
	q := m.store.Query(ctx, roleDeliverable).Where("parent_id = ?", taskID).Limit(maxListPage)
	out, err := specstore.List[Deliverable](m.store, q, roleDeliverable)
	if err != nil {
		return nil, fmt.Errorf("ListDeliverablesForTask: %w", err)
	}
	return out, nil
}

// ListAcceptanceCriteriaForTask returns all AcceptanceCriteria rows linked
// to taskID via has_acceptance_criteria (i.e. whose ParentID equals taskID).
func (m *taskManager) ListAcceptanceCriteriaForTask(ctx context.Context, taskID string) ([]AcceptanceCriteria, error) {
	q := m.store.Query(ctx, roleAcceptanceCriteria).Where("parent_id = ?", taskID).Limit(maxListPage)
	out, err := specstore.List[AcceptanceCriteria](m.store, q, roleAcceptanceCriteria)
	if err != nil {
		return nil, fmt.Errorf("ListAcceptanceCriteriaForTask: %w", err)
	}
	return out, nil
}

// WriteAcceptanceCriteriaResult writes the reviewer's result and
// result_notes onto an AcceptanceCriteria row.
func (m *taskManager) WriteAcceptanceCriteriaResult(ctx context.Context, criteriaID, result, notes string) error {
	res := m.store.Query(ctx, roleAcceptanceCriteria).Where("id = ?", criteriaID).
		Updates(map[string]any{
			"result":       result,
			"result_notes": notes,
			"updated_at":   time.Now().UTC().Format(time.RFC3339),
		})
	if res.Error != nil {
		return fmt.Errorf("WriteAcceptanceCriteriaResult: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrAcceptanceCriteriaNotFound
	}
	return nil
}
