package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
)

func (m *taskManager) requireMoveOn(ctx context.Context, requesterID string, backlogIDs ...string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil {
		return err
	}
	for _, id := range backlogIDs {
		if !visible.Moves(id) {
			return fmt.Errorf("%w: this needs Can move on the backlog", ErrBacklogForbidden)
		}
	}
	return nil
}

func (m *taskManager) requireMoveOnTask(ctx context.Context, requesterID, taskID string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	if !visible.Moves(task.Backlog) {
		return fmt.Errorf("%w: this needs Can move on the backlog the issue is in", ErrBacklogForbidden)
	}
	return nil
}

func (m *taskManager) requireSeeTask(ctx context.Context, requesterID, taskID string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	task, err := m.GetTask(ctx, taskID)
	if errors.Is(err, ErrTaskNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !visible.Sees(task.Backlog) {
		return fmt.Errorf("%w: that issue is in a backlog you cannot see", ErrBacklogForbidden)
	}
	return nil
}

func (m *taskManager) requireMoveOnEdgeSource(ctx context.Context, requesterID, fromID string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	task, err := m.GetTask(ctx, fromID)
	if errors.Is(err, ErrTaskNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !visible.Moves(task.Backlog) {
		return fmt.Errorf("%w: this needs Can move on the backlog the issue is in", ErrBacklogForbidden)
	}
	return nil
}

func (m *taskManager) firstBacklogID(ctx context.Context) (string, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return "", err
	}
	if len(backlogs) == 0 {
		return "", nil
	}
	return backlogs[0].ID, nil
}

func (m *taskManager) CreateTaskAs(ctx context.Context, requesterID string, task Task) (Task, error) {
	target := task.Backlog
	if target == "" {
		first, err := m.firstBacklogID(ctx)
		if err != nil {
			return Task{}, err
		}
		target = first
	}
	if err := m.requireMoveOn(ctx, requesterID, target); err != nil {
		return Task{}, err
	}
	return m.CreateTask(ctx, task)
}

func (m *taskManager) UpdateTaskAs(ctx context.Context, requesterID string, task Task) (Task, error) {
	if err := m.requireMoveOnTask(ctx, requesterID, task.ID); err != nil {
		return Task{}, err
	}
	if task.Backlog != "" {
		if err := m.requireMoveOn(ctx, requesterID, task.Backlog); err != nil {
			return Task{}, err
		}
	}
	return m.UpdateTask(ctx, task)
}

func (m *taskManager) MoveTaskToStateAs(ctx context.Context, requesterID, taskID, state string) (Task, error) {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return Task{}, err
	}
	return m.MoveTaskToState(ctx, taskID, state)
}

func (m *taskManager) PlaceTaskAs(ctx context.Context, requesterID, taskID, afterID, beforeID string) (Task, error) {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return Task{}, err
	}
	return m.PlaceTask(ctx, taskID, afterID, beforeID)
}

func (m *taskManager) DeleteTaskAs(ctx context.Context, requesterID, taskID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return err
	}
	return m.DeleteTask(ctx, taskID)
}

func (m *taskManager) CreateRelationshipAs(ctx context.Context, requesterID string, rel Relationship) (Relationship, error) {
	if err := m.requireMoveOnEdgeSource(ctx, requesterID, rel.FromID); err != nil {
		return Relationship{}, err
	}
	if err := m.requireSeeTask(ctx, requesterID, rel.ToID); err != nil {
		return Relationship{}, err
	}
	return m.CreateRelationship(ctx, rel)
}

func (m *taskManager) DeleteRelationshipAs(ctx context.Context, requesterID, fromID, toID, label string) error {
	if err := m.requireMoveOnEdgeSource(ctx, requesterID, fromID); err != nil {
		return err
	}
	return m.DeleteRelationship(ctx, fromID, toID, label)
}
