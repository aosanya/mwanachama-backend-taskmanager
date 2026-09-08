package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"
)

// AssignTask sets the Agent currently responsible for a Task by writing its
// AssignedAgentID column. A Task has at most one assignee — any prior value
// is simply overwritten.
//
// workflowRunID propagates the WorkflowRun anchor from the inbound event onto
// the Task per the chain-through rule:
//   - empty workflowRunID preserves the existing value
//   - non-empty + stored is empty: stored is set AND the started_task edge
//     from run→task is written
//   - non-empty + stored is the same: no-op
//   - non-empty + stored differs: [ErrWorkflowRunMismatch]
//
// If the task has any unmet outbound `depends_on` edges (i.e. depends on a
// source task that has not reached a terminal status), the assignment is
// still recorded but the task is transitioned to TaskStatusBlocked and the
// usual work.task.assigned dispatch is suppressed. When every blocking
// dependency eventually reaches a terminal status, [UnblockDependents] flips
// blocked → pending and re-fires work.task.assigned.
func (m *taskManager) AssignTask(ctx context.Context, taskID, agentID, workflowRunID string) error {
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	agent, err := m.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	// GetAgent accepts either the row ID or the AgentID slug; downstream
	// edge writes + the work.task.assigned payload need the row ID so
	// subscribers can resolve the agent without slug knowledge.
	resolvedAgentID := agent.ID

	// Apply chain-through before any writes so a mismatch fails fast
	// without partially mutating state.
	effectiveRunID := task.WorkflowRunID
	if workflowRunID != "" {
		switch task.WorkflowRunID {
		case "":
			if err := m.setTaskWorkflowRunID(ctx, taskID, workflowRunID); err != nil {
				return fmt.Errorf("AssignTask: set workflow_run_id: %w", err)
			}
			if err := m.LinkTaskToRun(ctx, workflowRunID, taskID); err != nil {
				return fmt.Errorf("AssignTask: link to run: %w", err)
			}
			effectiveRunID = workflowRunID
			task.WorkflowRunID = workflowRunID
		case workflowRunID:
			// no-op — same run, idempotent re-assign
		default:
			return ErrWorkflowRunMismatch
		}
	}

	if err := m.db.WithContext(ctx).Table(m.tables.Tasks).Where("id = ?", taskID).
		UpdateColumn("assigned_agent_id", resolvedAgentID).Error; err != nil {
		return fmt.Errorf("AssignTask: assign: %w", err)
	}

	// Check outbound depends_on edges. Any non-terminal target blocks dispatch.
	unmet, err := m.findUnmetDependencies(ctx, taskID)
	if err != nil {
		return fmt.Errorf("AssignTask: check deps: %w", err)
	}
	if len(unmet) > 0 {
		if task.Status.CanTransitionTo(TaskStatusBlocked) {
			if err := m.setTaskStatus(ctx, taskID, TaskStatusBlocked); err != nil {
				return fmt.Errorf("AssignTask: set blocked: %w", err)
			}
			m.publish(ctx, TopicTaskStatusChanged, TaskStatusChangedPayload{
				TaskID:        taskID,
				From:          task.Status,
				To:            TaskStatusBlocked,
				WorkflowRunID: effectiveRunID,
			})
		}
		// Suppress work.task.assigned — the AI must not run a task whose
		// dependencies have not landed. The assignment column is preserved
		// so an operator viewing the task sees the chosen agent.
		return nil
	}

	m.publish(ctx, TopicTaskAssigned, TaskAssignedPayload{
		TaskID:        taskID,
		AgentID:       resolvedAgentID,
		RoleName:      agent.RoleName,
		TaskCode:      task.TaskName,
		Title:         task.Title,
		Description:   task.Description,
		WorkflowRunID: effectiveRunID,
	})
	return nil
}

// setTaskWorkflowRunID updates only the workflow_run_id column, without
// touching other fields. Mirrors [setTaskStatus] for the chain-through
// path in AssignTask.
func (m *taskManager) setTaskWorkflowRunID(ctx context.Context, taskID, runID string) error {
	res := m.db.WithContext(ctx).Table(m.tables.Tasks).Where("id = ?", taskID).
		Updates(map[string]any{
			"workflow_run_id": runID,
			"updated_at":      time.Now().UTC().Format(time.RFC3339),
		})
	if res.Error != nil {
		return fmt.Errorf("setTaskWorkflowRunID: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// findUnmetDependencies returns the IDs of tasks this task depends on
// (via outbound depends_on edges) that have not reached a terminal status.
// The Y task has an outbound depends_on edge pointing to X when an import
// declares Y.depends_on includes X — so an outbound traversal yields the
// dependency targets that must complete first.
func (m *taskManager) findUnmetDependencies(ctx context.Context, taskID string) ([]string, error) {
	edges, err := m.TraverseRelationships(ctx, taskID, RelLabelDependsOn, DirectionOutbound)
	if err != nil {
		return nil, fmt.Errorf("findUnmetDependencies: %w", err)
	}
	var unmet []string
	for _, e := range edges {
		dep, err := m.GetTask(ctx, e.ToID)
		if err != nil {
			// A missing dep target is treated as unmet (defensive — the
			// import wrote the edge, but the entity was later deleted).
			unmet = append(unmet, e.ToID)
			continue
		}
		if !isTerminalStatus(dep.Status) {
			unmet = append(unmet, dep.ID)
		}
	}
	return unmet, nil
}

// setTaskStatus updates only the status column, leaving every other field
// untouched.
func (m *taskManager) setTaskStatus(ctx context.Context, taskID string, status TaskStatus) error {
	res := m.db.WithContext(ctx).Table(m.tables.Tasks).Where("id = ?", taskID).
		Updates(map[string]any{
			"status":     string(status),
			"updated_at": time.Now().UTC().Format(time.RFC3339),
		})
	if res.Error != nil {
		return fmt.Errorf("setTaskStatus: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrTaskNotFound
	}
	return nil
}

// UnassignTask clears the Task's AssignedAgentID column. Idempotent —
// returns nil whether or not an assignee was set.
func (m *taskManager) UnassignTask(ctx context.Context, taskID string) error {
	if _, err := m.GetTask(ctx, taskID); err != nil {
		return err
	}
	if err := m.db.WithContext(ctx).Table(m.tables.Tasks).Where("id = ?", taskID).
		UpdateColumn("assigned_agent_id", "").Error; err != nil {
		return fmt.Errorf("UnassignTask: %w", err)
	}
	return nil
}
