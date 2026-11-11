package mwanachamataskmanager

import "context"

func (m *taskManager) PatchTaskAs(ctx context.Context, requesterID string, patch TaskPatch) (Task, error) {
	if err := m.requireMoveOnTask(ctx, requesterID, patch.ID); err != nil {
		return Task{}, err
	}
	current, err := m.GetTask(ctx, patch.ID)
	if err != nil {
		return Task{}, err
	}
	if _, err := m.UpdateTask(ctx, applyPatch(current, patch)); err != nil {
		return Task{}, err
	}
	return m.GetTask(ctx, patch.ID)
}

func applyPatch(task Task, p TaskPatch) Task {
	if p.Title != nil {
		task.Title = *p.Title
	}
	if p.Description != nil {
		task.Description = *p.Description
	}
	if p.Status != nil {
		task.Status = *p.Status
	}
	if p.Priority != nil {
		task.Priority = *p.Priority
	}
	if p.Tags != nil {
		task.Tags = *p.Tags
	}
	if p.ProjectName != nil {
		task.ProjectName = *p.ProjectName
	}
	if p.DueAt != nil {
		task.DueAt = *p.DueAt
	}
	if p.EstimatedHours != nil {
		task.EstimatedHours = *p.EstimatedHours
	}
	if p.Context != nil {
		task.Context = *p.Context
	}
	return task
}
