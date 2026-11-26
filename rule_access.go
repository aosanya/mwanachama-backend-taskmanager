package mwanachamataskmanager

import (
	"context"
	"fmt"
)

func (v backlogVisibility) ReachesAny() bool {
	if v.all || v.reachOpen {
		return true
	}
	for _, reached := range v.reached {
		if reached {
			return true
		}
	}
	return false
}

func (v backlogVisibility) MovesAny() bool {
	if v.all {
		return true
	}
	for id := range v.levels {
		if v.Moves(id) {
			return true
		}
	}
	return false
}

func (m *taskManager) requireRuleAccess(ctx context.Context, requesterID string, write bool) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil {
		return err
	}
	if write {
		if visible.MovesAny() {
			return nil
		}
		return fmt.Errorf("%w: writing a rule needs Can move on a backlog", ErrBacklogForbidden)
	}
	if visible.ReachesAny() {
		return nil
	}
	return fmt.Errorf("%w: reading a rule needs the reach of a backlog", ErrBacklogForbidden)
}
