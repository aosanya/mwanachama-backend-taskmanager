// workflow_run_watchdog.go — stale-run detection helpers.
//
// This file adds the query helpers a watchdog sweeper uses, plus the
// handlers that process work.run.timeout and work.task.timeout events.
// mwanachama-backend-taskmanager has no scheduler of its own — the sweep
// loop and event dispatch live in whatever wires this package in.
package mwanachamataskmanager

import (
	"context"
	"time"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// ListWorkflowRunsStaleSince returns all non-terminal, unpaused WorkflowRuns
// whose last_event_at is before cutoff and whose timeout_published is false.
func (m *taskManager) ListWorkflowRunsStaleSince(ctx context.Context, cutoff time.Time) ([]WorkflowRun, error) {
	cutoffStr := cutoff.UTC().Format(time.RFC3339)
	var rows []gormstore.WorkflowRunRow
	err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).
		Where("(paused_at = '' OR paused_at IS NULL)").
		Where("timeout_published = ?", false).
		Where("last_event_at <> '' AND last_event_at < ?", cutoffStr).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	var out []WorkflowRun
	for _, r := range rows {
		run := gormstore.WorkflowRunFromRow(r)
		if run.Status.IsTerminal() {
			continue
		}
		out = append(out, run)
	}
	return out, nil
}

// ListWorkflowRunsStepStaleSince returns non-terminal, unpaused WorkflowRuns
// that have a current_step_id set and current_step_started_at before cutoff.
func (m *taskManager) ListWorkflowRunsStepStaleSince(ctx context.Context, cutoff time.Time) ([]WorkflowRun, error) {
	cutoffStr := cutoff.UTC().Format(time.RFC3339)
	var rows []gormstore.WorkflowRunRow
	err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).
		Where("(paused_at = '' OR paused_at IS NULL)").
		Where("current_step_id <> ''").
		Where("current_step_started_at <> '' AND current_step_started_at < ?", cutoffStr).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	var out []WorkflowRun
	for _, r := range rows {
		run := gormstore.WorkflowRunFromRow(r)
		if run.Status.IsTerminal() {
			continue
		}
		out = append(out, run)
	}
	return out, nil
}

// MarkTimeoutPublished sets timeout_published=true so the sweeper skips the
// run on subsequent ticks.
func (m *taskManager) MarkTimeoutPublished(ctx context.Context, runID string) error {
	return m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", runID).
		UpdateColumn("timeout_published", true).Error
}

// HandleRunTimeout processes a work.run.timeout event: flips the run to
// failed (if not already terminal) and cascades to non-terminal tasks.
func (m *taskManager) HandleRunTimeout(ctx context.Context, runID string) error {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		if err == ErrWorkflowRunNotFound {
			return nil
		}
		return err
	}
	if run.Status.IsTerminal() {
		return nil
	}

	if _, err := m.UpdateWorkflowRunStatus(ctx, runID, WorkflowRunStatusFailed, "stale"); err != nil {
		return err
	}

	tasks, _ := m.ListTasksForRun(ctx, runID)
	for _, t := range tasks {
		if t.Status == TaskStatusCompleted || t.Status == TaskStatusFailed || t.Status == TaskStatusCancelled {
			continue
		}
		t.Status = TaskStatusFailed
		if _, err := m.UpdateTask(ctx, t); err != nil {
			continue
		}
		m.publish(ctx, TopicTaskFailed, TaskFailedPayload{
			TaskID:        t.ID,
			Reason:        "run_timeout",
			WorkflowRunID: runID,
		})
	}
	return nil
}

// HandleTaskTimeout processes a work.task.timeout event: flips the task to failed.
func (m *taskManager) HandleTaskTimeout(ctx context.Context, taskOrTodoID string, runID string) error {
	task, err := m.GetTask(ctx, taskOrTodoID)
	if err != nil {
		if err == ErrTaskNotFound {
			return nil
		}
		return err
	}
	if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed || task.Status == TaskStatusCancelled {
		return nil
	}
	task.Status = TaskStatusFailed
	if _, err := m.UpdateTask(ctx, task); err != nil {
		return err
	}
	m.publish(ctx, TopicTaskFailed, TaskFailedPayload{
		TaskID:        taskOrTodoID,
		Reason:        "step_timeout",
		WorkflowRunID: runID,
	})
	return nil
}

// ListTasksForRun returns every Task linked to runID.
func (m *taskManager) ListTasksForRun(ctx context.Context, runID string) ([]Task, error) {
	return m.ListTasks(ctx, TaskFilter{WorkflowRunID: runID})
}
