package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const closedByWatchdog = "watchdog"

func (m *taskManager) BeginWorkflowRunStep(ctx context.Context, runID, stepCode string, limitMinutes int, cancelWorkflowCode string) (WorkflowRun, error) {
	if strings.TrimSpace(stepCode) == "" {
		return WorkflowRun{}, fmt.Errorf("%w: step_code is required", ErrInvalidTask)
	}
	if limitMinutes < 0 {
		return WorkflowRun{}, fmt.Errorf("%w: limit_minutes must not be negative", ErrInvalidTask)
	}
	if limitMinutes > 0 && cancelWorkflowCode == "" {
		return WorkflowRun{}, fmt.Errorf("%w: a step with a time limit names the cancel workflow to run when it passes", ErrInvalidTask)
	}
	if limitMinutes == 0 && cancelWorkflowCode != "" {
		return WorkflowRun{}, fmt.Errorf("%w: a cancel workflow needs a time limit to wait for", ErrInvalidTask)
	}
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if run.Status.IsTerminal() || run.Status == WorkflowRunStatusRollingBack {
		return WorkflowRun{}, fmt.Errorf("%w: status=%s", ErrCannotCancelTerminalRun, run.Status)
	}
	now := time.Now().UTC()
	run.CurrentStepID = stepCode
	run.CurrentStepStartedAt = now.Format(time.RFC3339)
	run.StepDueAt = ""
	run.StepCancelWorkflowCode = ""
	if limitMinutes > 0 {
		run.StepDueAt = now.Add(time.Duration(limitMinutes) * time.Minute).Format(time.RFC3339)
		run.StepCancelWorkflowCode = cancelWorkflowCode
	}
	run.UpdatedAt = run.CurrentStepStartedAt
	if err := m.saveRun(ctx, run); err != nil {
		return WorkflowRun{}, fmt.Errorf("BeginWorkflowRunStep: %w", err)
	}
	return run, nil
}

func (m *taskManager) AnswerWorkflowRunStep(ctx context.Context, runID string) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if run.StepDueAt == "" || run.Status.IsTerminal() {
		return run, nil
	}
	run.StepDueAt = ""
	run.StepCancelWorkflowCode = ""
	run.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := m.saveRun(ctx, run); err != nil {
		return WorkflowRun{}, fmt.Errorf("AnswerWorkflowRunStep: %w", err)
	}
	return run, nil
}

func (m *taskManager) CloseWorkflowRunsPastLimit(ctx context.Context, asOf string) ([]WorkflowRun, error) {
	at := time.Now().UTC()
	if asOf != "" {
		parsed, err := time.Parse(time.RFC3339, asOf)
		if err != nil {
			return nil, fmt.Errorf("%w: as_of must be an RFC 3339 timestamp", ErrInvalidTask)
		}
		at = parsed.UTC()
	}
	q := m.store.Query(ctx, roleWorkflowRun).
		Where("under_way = ?", true).
		Where("step_due_at <> '' AND step_due_at <= ?", at.Format(time.RFC3339)).
		Order("step_due_at ASC").
		Limit(maxListPage)
	due, err := specstore.List[WorkflowRun](m.store, q, roleWorkflowRun)
	if err != nil {
		return nil, fmt.Errorf("CloseWorkflowRunsPastLimit: %w", err)
	}
	var closed []WorkflowRun
	var failures []error
	for _, run := range due {
		done, err := m.closePastLimit(ctx, run, at)
		if err != nil {
			failures = append(failures, fmt.Errorf("run %s: %w", run.ID, err))
			continue
		}
		if done {
			closed = append(closed, run)
		}
	}
	return m.reread(ctx, closed), errors.Join(failures...)
}

func (m *taskManager) reread(ctx context.Context, runs []WorkflowRun) []WorkflowRun {
	out := make([]WorkflowRun, 0, len(runs))
	for _, run := range runs {
		if fresh, err := m.GetWorkflowRun(ctx, run.ID); err == nil {
			out = append(out, fresh)
		}
	}
	return out
}

func (m *taskManager) closePastLimit(ctx context.Context, run WorkflowRun, at time.Time) (bool, error) {
	if run.Status.IsTerminal() || run.Status == WorkflowRunStatusRollingBack || run.Status == WorkflowRunStatusCancelling {
		return false, nil
	}
	if run.StepCancelWorkflowCode == "" {
		return false, nil
	}
	cancelRun, err := m.openCancelRun(ctx, run)
	if err != nil {
		return false, err
	}
	reason := fmt.Sprintf("the time limit on step %s passed at %s with no answer", run.CurrentStepID, run.StepDueAt)
	now := at.Format(time.RFC3339)
	run.Status = WorkflowRunStatusClosed
	run.UpdatedAt = now
	run.CompletedAt = now
	run.CancelledBy = closedByWatchdog
	run.CancelReason = reason
	run.CancelRunID = cancelRun.ID
	if err := m.saveRun(ctx, run); err != nil {
		return false, fmt.Errorf("close: %w", err)
	}
	m.cascadeTaskCancellation(ctx, run.ID, reason)
	m.publishRunStatusEvent(ctx, run, at, reason)
	return true, nil
}

func (m *taskManager) openCancelRun(ctx context.Context, run WorkflowRun) (WorkflowRun, error) {
	name := run.Name + "-closed"
	opened, err := m.openWorkflowRun(ctx, WorkflowRun{
		Name:            name,
		TriggerEvent:    "step_limit_passed:" + run.ID,
		Initiator:       closedByWatchdog,
		WorkflowCode:    run.StepCancelWorkflowCode,
		WorkflowVersion: run.WorkflowVersion,
	})
	if errors.Is(err, ErrWorkflowRunNameExists) {
		return m.GetWorkflowRunByName(ctx, name)
	}
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("open cancel run: %w", err)
	}
	return opened, nil
}
