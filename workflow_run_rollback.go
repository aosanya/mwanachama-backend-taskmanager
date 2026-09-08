// workflow_run_rollback.go — WorkflowRun rollback coordinator and artifact cleanup.
//
// RollbackWorkflowRun orchestrates the compensation sequence. Cross-service
// compensation is event-driven (each service handles its own compensation on
// receipt of work.run.rolling_back) — no direct service-to-service calls, so
// no client stubs are needed here.
//
// DeleteWorkflowRunArtifacts is this package's own compensation leg: it
// resets every Task anchored by the run ID to pending (clearing
// workflow_run_id and completed_at, keeping all other task fields and
// non-run edges intact — clearing workflow_run_id is also the started_task
// edge's own removal, since that edge IS the column), and soft-deletes
// every TaskTodo anchored to the run. Its guard check, every task reset, and
// every todo delete run inside one [gorm.DB.Transaction] — the "known gap"
// the pre-GORM implementation could not close (entitygraph.DataManager had
// no cross-call transaction primitive) is closed here.
package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// RollbackWorkflowRun implements [TaskManager.RollbackWorkflowRun].
// Sequence: rolling_back → compensate cross-service artifacts → compensate own artifacts → rolled_back (or rollback_failed).
func (m *taskManager) RollbackWorkflowRun(ctx context.Context, runID, reason string) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if run.Status == WorkflowRunStatusRollingBack {
		return WorkflowRun{}, ErrRollbackConflict
	}
	if !run.Status.CanTransitionTo(WorkflowRunStatusRollingBack) {
		return WorkflowRun{}, fmt.Errorf("%w: %s → rolling_back", ErrInvalidRunStatusTransition, run.Status)
	}

	// Step 1 — acquire the rolling_back lock (durably committed; not part of
	// step 3's transaction — see file doc).
	rollingRun, err := m.UpdateWorkflowRunStatus(ctx, runID, WorkflowRunStatusRollingBack, reason)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("RollbackWorkflowRun: acquire: %w", err)
	}

	// Step 2 — cross-service compensation (event-driven; see file doc).

	// Step 3 — this package's own artifact cleanup.
	if err := m.DeleteWorkflowRunArtifacts(ctx, runID); err != nil {
		var rollbackErr error
		if errors.Is(err, ErrForeignRunDependency) {
			rollbackErr = err
		} else {
			rollbackErr = fmt.Errorf("delete artifacts: %w", err)
		}
		failedRun, ferr := m.UpdateWorkflowRunStatus(ctx, runID, WorkflowRunStatusRollbackFailed, rollbackErr.Error())
		if ferr != nil {
			return rollingRun, rollbackErr
		}
		return failedRun, rollbackErr
	}

	// Step 4 — finalize.
	finalRun, err := m.UpdateWorkflowRunStatus(ctx, runID, WorkflowRunStatusRolledBack, reason)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("RollbackWorkflowRun: finalize: %w", err)
	}
	return finalRun, nil
}

// DeleteWorkflowRunArtifacts implements [TaskManager.DeleteWorkflowRunArtifacts].
// Tasks are reset to pending (not deleted). TaskTodos are soft-deleted.
func (m *taskManager) DeleteWorkflowRunArtifacts(ctx context.Context, runID string) error {
	if _, err := m.GetWorkflowRun(ctx, runID); err != nil {
		return err
	}

	var resetTaskIDs []string
	txErr := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tasks []gormstore.TaskRow
		if err := tx.Table(m.tables.Tasks).Where("workflow_run_id = ? AND deleted = ?", runID, false).Find(&tasks).Error; err != nil {
			return fmt.Errorf("list tasks: %w", err)
		}

		// Guard: check for Tasks in this run that are depended on by Tasks in OTHER runs.
		for _, task := range tasks {
			var inbound []gormstore.TaskDependencyRow
			if err := tx.Table(m.tables.TaskDependencies).Where("to_task_id = ?", task.ID).Find(&inbound).Error; err != nil {
				return fmt.Errorf("list inbound deps for %s: %w", task.ID, err)
			}
			for _, rel := range inbound {
				var fromTask gormstore.TaskRow
				if err := tx.Table(m.tables.Tasks).Where("id = ?", rel.FromTaskID).First(&fromTask).Error; err != nil {
					continue // missing task — not a blocker
				}
				if fromTask.WorkflowRunID != "" && fromTask.WorkflowRunID != runID {
					return fmt.Errorf("%w: task %s is depended on by task %s (run %s)",
						ErrForeignRunDependency, task.ID, fromTask.ID, fromTask.WorkflowRunID)
				}
			}
		}

		// Reset Tasks: status → pending, clear workflow_run_id + completed_at.
		// All other task fields and edges (member_of, has_tag, blocks,
		// depends_on) are preserved. Clearing workflow_run_id is itself the
		// started_task edge's removal — see file doc.
		now := time.Now().UTC().Format(time.RFC3339)
		for _, task := range tasks {
			if err := tx.Table(m.tables.Tasks).Where("id = ?", task.ID).Updates(map[string]any{
				"status":          string(TaskStatusPending),
				"workflow_run_id": "",
				"completed_at":    "",
				"updated_at":      now,
			}).Error; err != nil {
				return fmt.Errorf("reset task %s: %w", task.ID, err)
			}
			resetTaskIDs = append(resetTaskIDs, task.ID)
		}

		// Soft-delete TaskTodos anchored to this run (ephemeral decomposition
		// artifacts) — deleting the row also drops its has_todo/started_todo/
		// todo_assigned_to columns, so no separate edge cleanup is needed.
		if err := tx.Table(m.tables.TaskTodos).Where("workflow_run_id = ? AND deleted = ?", runID, false).
			UpdateColumn("deleted", true).Error; err != nil {
			return fmt.Errorf("delete todos: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return fmt.Errorf("DeleteWorkflowRunArtifacts: %w", txErr)
	}

	for _, taskID := range resetTaskIDs {
		m.publishTaskRolledBack(ctx, taskID, runID)
	}
	return nil
}

// publishTaskRolledBack emits work.task.rolled_back for observability after a Task is rolled back.
func (m *taskManager) publishTaskRolledBack(ctx context.Context, taskID, runID string) {
	m.publish(ctx, TopicTaskRolledBack, TaskRolledBackPayload{TaskID: taskID, WorkflowRunID: runID})
}
