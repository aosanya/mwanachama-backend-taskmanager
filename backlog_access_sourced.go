package mwanachamataskmanager

import "context"

func (m *taskManager) requireMoveOnFirstBacklog(ctx context.Context, requesterID string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	first, err := m.firstBacklogID(ctx)
	if err != nil {
		return err
	}
	return m.requireMoveOn(ctx, requesterID, first)
}

func (m *taskManager) OpenSourcedTasksAs(ctx context.Context, requesterID string, req OpenSourcedTasks) (SourcedTasksOpened, error) {
	if err := m.requireMoveOnFirstBacklog(ctx, requesterID); err != nil {
		return SourcedTasksOpened{}, err
	}
	return m.OpenSourcedTasks(ctx, req)
}

func (m *taskManager) GenerateTasksAs(ctx context.Context, requesterID string, req GenerateTasks) (TasksGenerated, error) {
	if err := m.requireMoveOnFirstBacklog(ctx, requesterID); err != nil {
		return TasksGenerated{}, err
	}
	return m.GenerateTasks(ctx, req)
}
