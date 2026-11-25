package mwanachamataskmanager

import (
	"context"
	"fmt"
)

func (m *taskManager) requireRuleAccess(ctx context.Context, requesterID string, write bool) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	for id := range visible.levels {
		if !write || visible.Moves(id) {
			return nil
		}
	}
	if write {
		return fmt.Errorf("%w: writing a rule needs Can move on a backlog", ErrBacklogForbidden)
	}
	return fmt.Errorf("%w: reading a rule needs Can see on a backlog", ErrBacklogForbidden)
}
