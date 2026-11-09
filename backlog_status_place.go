package mwanachamataskmanager

import "context"

func (m *taskManager) placeFor(ctx context.Context, backlogID, stepID string, status TaskStatus) (string, string, error) {
	if backlogID == "" {
		return "", "", nil
	}
	return m.placeAfterStatusChange(ctx, Task{Backlog: backlogID, Step: stepID, Status: status})
}

func (m *taskManager) placeColumnsFor(ctx context.Context, taskID string, status TaskStatus) (map[string]any, error) {
	backlogID, stepID := m.storedPlace(ctx, taskID)
	if backlogID == "" {
		return nil, nil
	}
	backlogID, stepID, err := m.placeFor(ctx, backlogID, stepID, status)
	if err != nil {
		return nil, err
	}
	return map[string]any{"backlog": backlogID, "step": stepID}, nil
}
