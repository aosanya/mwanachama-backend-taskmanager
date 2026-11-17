package mwanachamataskmanager

import (
	"context"
	"time"
)

func (m *taskManager) placeFor(ctx context.Context, backlogID, stepID string, status TaskStatus) (string, string, error) {
	if backlogID == "" {
		return "", "", nil
	}
	return m.placeAfterStatusChange(ctx, Task{Backlog: backlogID, Step: stepID, Status: status})
}

func (m *taskManager) placeColumnsFor(ctx context.Context, taskID string, status TaskStatus) (map[string]any, error) {
	before, stepBefore := m.storedPlace(ctx, taskID)
	if before == "" {
		return nil, nil
	}
	backlogID, stepID, err := m.placeFor(ctx, before, stepBefore, status)
	if err != nil {
		return nil, err
	}
	columns := map[string]any{"backlog": backlogID, "step": stepID}
	if backlogID != before || stepID != stepBefore {
		columns["step_entered_at"] = time.Now().UTC().Format(time.RFC3339)
	}
	return columns, nil
}
