// workflow_run_cancel.go — Mid-flight WorkflowRun cancel.
//
// CancelWorkflowRun handles the synchronous half of the cancel flow: it flips
// the run to the transient cancelling state, persists the cancellation
// envelope (cancelled_by, cancel_reason, cancelling_until), cascades
// work.task.cancelled to every non-terminal Task anchored by the run, and
// publishes work.run.cancelling.
//
// FinalizeWorkflowRunCancellation is the deferred half: after the quiesce
// deadline elapses (driven by the caller's own goroutine — this package has
// no scheduler of its own), it transitions the run from cancelling →
// cancelled and publishes work.run.cancelled. Both halves are idempotent
// and safe to call multiple times.
package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// CancelWorkflowRun implements [TaskManager.CancelWorkflowRun].
func (m *taskManager) CancelWorkflowRun(ctx context.Context, runID, reason, cancelledBy string, quiesceDeadline time.Time) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRun{}, err
	}

	if run.Status == WorkflowRunStatusCancelling {
		return run, nil
	}
	if run.Status != WorkflowRunStatusInProgress {
		return WorkflowRun{}, fmt.Errorf("%w: status=%s", ErrCannotCancelTerminalRun, run.Status)
	}

	now := time.Now().UTC()
	run.Status = WorkflowRunStatusCancelling
	run.UpdatedAt = now.Format(time.RFC3339)
	run.CancelledBy = cancelledBy
	run.CancelReason = reason
	run.CancellingUntil = quiesceDeadline.UTC().Format(time.RFC3339)

	row := gormstore.WorkflowRunToRow(run)
	if err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", runID).Save(&row).Error; err != nil {
		return WorkflowRun{}, fmt.Errorf("CancelWorkflowRun: %w", err)
	}
	result := gormstore.WorkflowRunFromRow(row)

	// Cascade: flip every non-terminal Task anchored by the run to cancelled
	// and emit work.task.cancelled per task. Failures cascading individual
	// tasks are logged but do not abort the cancel.
	m.cascadeTaskCancellation(ctx, runID, reason)

	m.publishRunStatusEvent(ctx, result, now, reason)
	return result, nil
}

// FinalizeWorkflowRunCancellation implements
// [TaskManager.FinalizeWorkflowRunCancellation].
func (m *taskManager) FinalizeWorkflowRunCancellation(ctx context.Context, runID string) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if run.Status != WorkflowRunStatusCancelling {
		return run, nil
	}

	now := time.Now().UTC()
	run.Status = WorkflowRunStatusCancelled
	run.UpdatedAt = now.Format(time.RFC3339)
	run.CompletedAt = run.UpdatedAt

	row := gormstore.WorkflowRunToRow(run)
	if err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", runID).Save(&row).Error; err != nil {
		return WorkflowRun{}, fmt.Errorf("FinalizeWorkflowRunCancellation: %w", err)
	}
	result := gormstore.WorkflowRunFromRow(row)

	m.publishRunStatusEvent(ctx, result, now, result.CancelReason)
	return result, nil
}

// cascadeTaskCancellation flips every non-terminal Task whose workflow_run_id
// matches runID to cancelled and emits work.task.cancelled per affected task.
// Best-effort: per-task failures are logged and do not abort the cascade.
func (m *taskManager) cascadeTaskCancellation(ctx context.Context, runID, reason string) {
	tasks, err := m.ListTasks(ctx, TaskFilter{WorkflowRunID: runID})
	if err != nil {
		slog.ErrorContext(ctx, "CancelWorkflowRun: list tasks for cascade", "run_id", runID, "err", err)
		return
	}
	for _, t := range tasks {
		if isTerminalStatus(t.Status) {
			continue
		}
		if err := m.cancelTask(ctx, t, reason); err != nil {
			slog.ErrorContext(ctx, "CancelWorkflowRun: cancel task", "task_id", t.ID, "run_id", runID, "err", err)
		}
	}
}

// cancelTask transitions a single Task to cancelled and publishes
// work.task.cancelled. Returns nil even if the state-machine rejects the
// transition (handled by the caller); a real persistence error is surfaced.
func (m *taskManager) cancelTask(ctx context.Context, task Task, reason string) error {
	if !task.Status.CanTransitionTo(TaskStatusCancelled) {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	task.Status = TaskStatusCancelled
	task.UpdatedAt = now
	if task.CompletedAt == "" {
		task.CompletedAt = now
	}
	row := gormstore.TaskToRow(task)
	res := m.db.WithContext(ctx).Table(m.tables.Tasks).Where("id = ?", task.ID).Save(&row)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("cancelTask: %w", res.Error)
	}
	m.publish(ctx, TopicTaskCancelled, TaskCancelledPayload{
		TaskID:        task.ID,
		WorkflowRunID: task.WorkflowRunID,
		Reason:        reason,
	})
	return nil
}
