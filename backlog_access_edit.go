package mwanachamataskmanager

import (
	"context"
	"fmt"
)

func (m *taskManager) requireEditOn(ctx context.Context, requesterID string, backlogIDs ...string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil {
		return err
	}
	for _, id := range backlogIDs {
		if !visible.Edits(id) {
			return fmt.Errorf("%w: this needs Can edit on the backlog", ErrBacklogForbidden)
		}
	}
	return nil
}

func (m *taskManager) RenameBacklogAs(ctx context.Context, requesterID, backlogID, name string) (BacklogSummary, error) {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return BacklogSummary{}, err
	}
	return m.RenameBacklog(ctx, backlogID, name)
}

func (m *taskManager) DeleteBacklogAs(ctx context.Context, requesterID, backlogID string) error {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return err
	}
	return m.DeleteBacklog(ctx, backlogID)
}

func (m *taskManager) AddBacklogStepAs(ctx context.Context, requesterID, backlogID, name, after string) (BacklogSummary, error) {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return BacklogSummary{}, err
	}
	return m.AddBacklogStep(ctx, backlogID, name, after)
}

func (m *taskManager) RenameBacklogStepAs(ctx context.Context, requesterID, backlogID, stepID, name string) (BacklogSummary, error) {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return BacklogSummary{}, err
	}
	return m.RenameBacklogStep(ctx, backlogID, stepID, name)
}

func (m *taskManager) ReorderBacklogStepsAs(ctx context.Context, requesterID, backlogID string, steps []string) (BacklogSummary, error) {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return BacklogSummary{}, err
	}
	return m.ReorderBacklogSteps(ctx, backlogID, steps)
}

func (m *taskManager) RemoveBacklogStepAs(ctx context.Context, requesterID, backlogID, stepID string) (BacklogSummary, error) {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return BacklogSummary{}, err
	}
	return m.RemoveBacklogStep(ctx, backlogID, stepID)
}

func (m *taskManager) ReplaceBacklogsAs(ctx context.Context, requesterID string, backlogs []Backlog) (BacklogList, error) {
	existing, err := m.backlogs(ctx)
	if err != nil {
		return BacklogList{}, err
	}
	ids := make([]string, 0, len(existing))
	for _, b := range existing {
		ids = append(ids, b.ID)
	}
	if err := m.requireEditOn(ctx, requesterID, ids...); err != nil {
		return BacklogList{}, err
	}
	return m.ReplaceBacklogs(ctx, backlogs)
}

func (m *taskManager) GiveBacklogAccessAs(ctx context.Context, requesterID, backlogID, groupID, level string) (BacklogAccess, error) {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return BacklogAccess{}, err
	}
	return m.GiveBacklogAccess(ctx, backlogID, groupID, level)
}

func (m *taskManager) TakeBacklogAccessAs(ctx context.Context, requesterID, backlogID, groupID string) (BacklogAccess, error) {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return BacklogAccess{}, err
	}
	return m.TakeBacklogAccess(ctx, backlogID, groupID)
}
