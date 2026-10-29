package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func (m *taskManager) StartWorkflowRun(ctx context.Context, name, workflowCode string, workflowVersion int, triggerEvent, initiator string) (WorkflowRun, error) {
	if workflowCode == "" {
		return WorkflowRun{}, fmt.Errorf("%w: workflow_code is required", ErrInvalidTask)
	}
	if workflowVersion < 1 {
		return WorkflowRun{}, fmt.Errorf("%w: workflow_version must be 1 or more", ErrInvalidTask)
	}
	return m.openWorkflowRun(ctx, WorkflowRun{
		Name:            name,
		TriggerEvent:    triggerEvent,
		Initiator:       initiator,
		WorkflowCode:    workflowCode,
		WorkflowVersion: workflowVersion,
	})
}

func (m *taskManager) ListWorkflowRunsUnderWay(ctx context.Context, workflowCode string) ([]WorkflowRun, error) {
	q := m.store.Query(ctx, roleWorkflowRun).Where("under_way = ?", true)
	if workflowCode != "" {
		q = q.Where("workflow_code = ?", workflowCode)
	}
	out, err := specstore.List[WorkflowRun](m.store, q.Order("created_at DESC").Limit(maxListPage), roleWorkflowRun)
	if err != nil {
		return nil, fmt.Errorf("ListWorkflowRunsUnderWay: %w", err)
	}
	return out, nil
}

func (m *taskManager) StopWorkflowRun(ctx context.Context, runID, stoppedBy, reason string, restartOnVersion int) (WorkflowRunStop, error) {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRunStop{}, err
	}
	if run.Status.IsTerminal() || run.Status == WorkflowRunStatusRollingBack {
		return WorkflowRunStop{}, fmt.Errorf("%w: status=%s", ErrCannotCancelTerminalRun, run.Status)
	}
	if restartOnVersion < 0 {
		return WorkflowRunStop{}, fmt.Errorf("%w: restart_on_version must be 1 or more", ErrInvalidTask)
	}
	if restartOnVersion > 0 && run.WorkflowCode == "" {
		return WorkflowRunStop{}, fmt.Errorf("%w: run %s names no workflow to start again", ErrInvalidTask, run.ID)
	}

	now := time.Now().UTC()
	run.Status = WorkflowRunStatusCancelled
	run.UnderWay = false
	run.UpdatedAt = now.Format(time.RFC3339)
	run.CompletedAt = run.UpdatedAt
	run.CancelledBy = stoppedBy
	run.CancelReason = reason
	if err := m.saveRun(ctx, run); err != nil {
		return WorkflowRunStop{}, fmt.Errorf("StopWorkflowRun: %w", err)
	}
	m.cascadeTaskCancellation(ctx, run.ID, reason)
	m.publishRunStatusEvent(ctx, run, now, reason)

	stop := WorkflowRunStop{Stopped: run}
	if restartOnVersion == 0 {
		return stop, nil
	}
	restarted, err := m.openWorkflowRun(ctx, WorkflowRun{
		TriggerEvent:       run.TriggerEvent,
		Initiator:          run.Initiator,
		WorkflowCode:       run.WorkflowCode,
		WorkflowVersion:    restartOnVersion,
		RestartedFromRunID: run.ID,
	})
	if err != nil {
		return stop, fmt.Errorf("StopWorkflowRun: start again: %w", err)
	}
	stop.Restarted = &restarted
	return stop, nil
}
