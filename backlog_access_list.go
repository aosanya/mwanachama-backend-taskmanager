package mwanachamataskmanager

import (
	"context"

	"gorm.io/gorm"
)

func (m *taskManager) narrowToVisible(ctx context.Context, q *gorm.DB, requesterID string) (*gorm.DB, error) {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return q, err
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return nil, err
	}
	var ids, existing []string
	unplacedVisible := false
	for i, b := range backlogs {
		existing = append(existing, b.ID)
		if !visible.Sees(b.ID) {
			continue
		}
		ids = append(ids, b.ID)
		unplacedVisible = unplacedVisible || i == 0
	}
	switch {
	case len(ids) == 0:
		return q.Where("1 = 0"), nil
	case unplacedVisible:
		return q.Where("(backlog IN ? OR backlog = '' OR backlog IS NULL OR backlog NOT IN ?)", ids, existing), nil
	}
	return q.Where("backlog IN ?", ids), nil
}
