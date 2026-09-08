// workflow_run_failure_budget.go — recovery-pipeline failure budget.
//
// A WorkflowRun's failure_pipeline_budget caps the number of recovery
// pipeline activations dispatched under the run's lineage. The counter
// lives on the root run; child (recovery) runs reference their root via
// root_workflow_run_id.
package mwanachamataskmanager

import (
	"context"
	"encoding/json"
	"fmt"
)

// SetFailureBudget locks the failure-pipeline budget on a root WorkflowRun.
// Refuses to overwrite a non-zero value (the budget is frozen for the
// lifetime of the run) and refuses to act on non-root runs.
func (m *taskManager) SetFailureBudget(ctx context.Context, runID string, budget int) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if run.ParentWorkflowRunID != "" {
		return WorkflowRun{}, fmt.Errorf("%w: run %s has parent %s", ErrNotRootWorkflowRun, runID, run.ParentWorkflowRunID)
	}
	if run.FailurePipelineBudget != 0 {
		return WorkflowRun{}, fmt.Errorf("%w: run %s has budget %d", ErrFailureBudgetAlreadySet, runID, run.FailurePipelineBudget)
	}
	if err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", runID).
		Updates(map[string]any{
			"failure_pipeline_budget": budget,
			// Root runs default their root pointer to their own id so
			// downstream services can treat root_workflow_run_id uniformly.
			"root_workflow_run_id": runID,
		}).Error; err != nil {
		return WorkflowRun{}, fmt.Errorf("SetFailureBudget: %w", err)
	}
	return m.GetWorkflowRun(ctx, runID)
}

// IncrementFailureBudget atomically increments failure_pipelines_used on
// the root run identified by rootRunID. The child_run_id is recorded in
// counted_child_run_ids; subsequent calls with the same child_run_id are
// idempotent (they return the current counter state without incrementing).
//
// The returned `exhausted` flag is true iff `used > budget` after the
// increment.
func (m *taskManager) IncrementFailureBudget(ctx context.Context, rootRunID, childRunID string) (used, budget int, exhausted bool, err error) {
	run, getErr := m.GetWorkflowRun(ctx, rootRunID)
	if getErr != nil {
		return 0, 0, false, getErr
	}
	if run.ParentWorkflowRunID != "" {
		return 0, 0, false, fmt.Errorf("%w: run %s has parent %s", ErrNotRootWorkflowRun, rootRunID, run.ParentWorkflowRunID)
	}

	for _, id := range run.CountedChildRunIDs {
		if id == childRunID {
			return run.FailurePipelinesUsed, run.FailurePipelineBudget, exhaustedAt(run.FailurePipelinesUsed, run.FailurePipelineBudget), nil
		}
	}

	newUsed := run.FailurePipelinesUsed + 1
	newCounted := append(append([]string(nil), run.CountedChildRunIDs...), childRunID)
	countedJSON, err := json.Marshal(newCounted)
	if err != nil {
		return 0, 0, false, fmt.Errorf("IncrementFailureBudget: %w", err)
	}

	if updateErr := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", rootRunID).
		Updates(map[string]any{
			"failure_pipelines_used": newUsed,
			"counted_child_run_ids":  countedJSON,
		}).Error; updateErr != nil {
		return 0, 0, false, fmt.Errorf("IncrementFailureBudget: %w", updateErr)
	}
	result, getErr := m.GetWorkflowRun(ctx, rootRunID)
	if getErr != nil {
		return 0, 0, false, fmt.Errorf("IncrementFailureBudget: reread: %w", getErr)
	}
	return result.FailurePipelinesUsed, result.FailurePipelineBudget, exhaustedAt(result.FailurePipelinesUsed, result.FailurePipelineBudget), nil
}

// exhaustedAt reports whether the post-increment counter exceeds the cap.
// A zero budget means "unconfigured" — the gate is open until the budget
// is set, so exhaustion never trips. Once a positive budget is set,
// `used > budget` trips the gate.
func exhaustedAt(used, budget int) bool {
	if budget <= 0 {
		return false
	}
	return used > budget
}

// CreateRecoveryWorkflowRun mints a child WorkflowRun spawned by a parent
// run's failure. The child carries parent_workflow_run_id and
// root_workflow_run_id but no budget of its own — callers always read and
// increment the root's counter via [IncrementFailureBudget].
func (m *taskManager) CreateRecoveryWorkflowRun(ctx context.Context, name, triggerEvent, initiator, parentRunID, rootRunID string) (WorkflowRun, error) {
	if parentRunID == "" {
		return WorkflowRun{}, fmt.Errorf("%w: parent_workflow_run_id is required for a recovery run", ErrInvalidTask)
	}
	if rootRunID == "" {
		parent, err := m.GetWorkflowRun(ctx, parentRunID)
		if err != nil {
			return WorkflowRun{}, err
		}
		rootRunID = parent.RootWorkflowRunID
		if rootRunID == "" {
			rootRunID = parent.ID
		}
	}
	run, err := m.CreateWorkflowRun(ctx, name, triggerEvent, initiator)
	if err != nil {
		return WorkflowRun{}, err
	}
	if err := m.db.WithContext(ctx).Table(m.tables.WorkflowRuns).Where("id = ?", run.ID).
		Updates(map[string]any{
			"parent_workflow_run_id": parentRunID,
			"root_workflow_run_id":   rootRunID,
		}).Error; err != nil {
		return WorkflowRun{}, fmt.Errorf("CreateRecoveryWorkflowRun: stamp parentage: %w", err)
	}
	return m.GetWorkflowRun(ctx, run.ID)
}
