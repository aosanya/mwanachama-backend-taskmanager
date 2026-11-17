package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"
)

type HeartbeatRequest struct {
	TaskID       string `json:"task_id"`
	Agent        string `json:"agent"`
	LeaseMinutes int    `json:"lease_minutes"`
}

type LeaseRenewed struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	Agent      string `json:"agent"`
	LeaseUntil string `json:"lease_until"`
}

func (m *taskManager) Heartbeat(ctx context.Context, requesterID string, req HeartbeatRequest) (LeaseRenewed, error) {
	length, err := leaseLength(req.LeaseMinutes)
	if err != nil {
		return LeaseRenewed{}, err
	}
	released, err := m.releaseExpiredLeases(ctx)
	if err != nil {
		return LeaseRenewed{}, fmt.Errorf("Heartbeat: %w", err)
	}
	task, err := m.GetTaskFor(ctx, requesterID, req.TaskID)
	if err != nil {
		return LeaseRenewed{}, err
	}
	if err := m.requireMoveOnTask(ctx, requesterID, task.ID); err != nil {
		return LeaseRenewed{}, err
	}
	agent := agentOrCaller(req.Agent, requesterID)
	if reason := heldRefusal(task, agent, released); reason != "" {
		return LeaseRenewed{}, fmt.Errorf("%w: %s", ErrLeaseNotHeld, reason)
	}
	now := time.Now().UTC()
	until := leaseEnd(now, length)
	res := m.store.Query(ctx, roleTask).
		Where("id = ? AND deleted = ? AND status = ? AND assigned_to = ? AND lease_until = ?", task.ID, false, string(TaskStatusInProgress), agent, task.LeaseUntil).
		Updates(map[string]any{"lease_until": until, "updated_at": now.Format(time.RFC3339)})
	if res.Error != nil {
		return LeaseRenewed{}, fmt.Errorf("Heartbeat: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return LeaseRenewed{}, fmt.Errorf("%w: the issue changed hands while the lease was renewed", ErrLeaseNotHeld)
	}
	return LeaseRenewed{ID: task.ID, Code: task.Code, Agent: agent, LeaseUntil: until}, nil
}

func heldRefusal(task Task, agent string, released map[string]leaseRow) string {
	if row, ran := released[task.ID]; ran {
		return fmt.Sprintf("the lease of %s ran out and the issue went back to To do", row.AssignedTo)
	}
	switch {
	case task.Status != TaskStatusInProgress:
		return "the issue is not in progress"
	case task.LeaseUntil == "":
		return "the issue was taken without a lease"
	case task.AssignedTo != agent:
		return fmt.Sprintf("the issue is held by %q, not %q", task.AssignedTo, agent)
	}
	return ""
}
