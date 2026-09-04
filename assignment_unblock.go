package mwanachamataskmanager

import (
	"context"
	"fmt"
	"log"
	"time"
)

// UnblockDependents implements the inverse trigger of [taskManager.AssignTask]:
// whenever completedTaskID reaches a terminal status, walk every Task with an
// inbound `depends_on` edge from it and flip blocked → pending (and re-fire
// [TopicTaskAssigned]) for the ones whose entire `depends_on` set is now
// satisfied. Tasks that have lost their cached assignee or that still have
// other unmet dependencies are left blocked.
//
// Called by the work.task.completed handler. Idempotent: a dependent already
// in a non-blocked state is skipped, so redelivery and self-loop receipts are
// safely no-op.
func (m *taskManager) UnblockDependents(ctx context.Context, completedTaskID string) error {
	if _, err := m.GetTask(ctx, completedTaskID); err != nil {
		return err
	}
	edges, err := m.TraverseRelationships(ctx, completedTaskID, RelLabelDependsOn, DirectionInbound)
	if err != nil {
		return fmt.Errorf("UnblockDependents: traverse depends_on: %w", err)
	}
	for _, e := range edges {
		m.maybeUnblockDependent(ctx, completedTaskID, e.FromID)
	}
	return nil
}

// maybeUnblockDependent evaluates a single dependent task and, if every
// gating condition is satisfied, transitions it pending and republishes
// work.task.assigned. Failures are logged; the loop in UnblockDependents
// continues so one bad dependent does not stall the rest.
func (m *taskManager) maybeUnblockDependent(ctx context.Context, completedTaskID, dependentID string) {
	dependent, err := m.GetTask(ctx, dependentID)
	if err != nil {
		log.Printf("mwanachamataskmanager: UnblockDependents: GetTask %s: %v", dependentID, err)
		return
	}
	if dependent.Status != TaskStatusBlocked {
		return
	}
	unmet, err := m.findUnmetDependencies(ctx, dependentID)
	if err != nil {
		log.Printf("mwanachamataskmanager: UnblockDependents: findUnmetDependencies %s: %v", dependentID, err)
		return
	}
	if len(unmet) > 0 {
		return
	}
	assignedEdges, err := m.TraverseRelationships(ctx, dependentID, RelLabelAssignedTo, DirectionOutbound)
	if err != nil {
		log.Printf("mwanachamataskmanager: UnblockDependents: traverse assigned_to %s: %v", dependentID, err)
		return
	}
	if len(assignedEdges) == 0 {
		// No cached assignee — leave blocked; nothing to dispatch.
		return
	}
	agentEntityID := assignedEdges[0].ToID
	agent, err := m.GetAgent(ctx, agentEntityID)
	if err != nil {
		log.Printf("mwanachamataskmanager: UnblockDependents: GetAgent %s: %v", agentEntityID, err)
		return
	}
	if err := m.setTaskStatus(ctx, dependentID, TaskStatusPending); err != nil {
		log.Printf("mwanachamataskmanager: UnblockDependents: setTaskStatus %s pending: %v", dependentID, err)
		return
	}
	m.publish(ctx, TopicTaskStatusChanged, TaskStatusChangedPayload{
		TaskID: dependentID,
		From:   TaskStatusBlocked,
		To:     TaskStatusPending,
	})
	m.publish(ctx, TopicTaskAssigned, TaskAssignedPayload{
		TaskID:      dependentID,
		AgentID:     agentEntityID,
		RoleName:    agent.RoleName,
		TaskCode:    dependent.TaskName,
		Title:       dependent.Title,
		Description: dependent.Description,
	})
	log.Printf("mwanachamataskmanager: UnblockDependents: task %s blocked→pending (dep %s satisfied)",
		dependentID, completedTaskID)
}

// UnblockTask transitions a task from the `blocked` status (entered when an
// operator chose the mark-blocked direction option) back to
// `awaiting-direction` so the direction form can be re-opened for a new
// resolution cycle. Appends note to the task description when non-empty.
//
// Returns [ErrInvalidStatusTransition] if the task is not currently blocked.
func (m *taskManager) UnblockTask(ctx context.Context, taskID, note string) (Task, error) {
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return Task{}, err
	}
	if !task.Status.CanTransitionTo(TaskStatusAwaitingDirection) {
		return Task{}, fmt.Errorf("%w: %s → awaiting-direction", ErrInvalidStatusTransition, task.Status)
	}
	if note != "" {
		task.Description = task.Description + "\n\nUnblocked: " + note
	}
	task.Status = TaskStatusAwaitingDirection
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	updated, err := m.UpdateTask(ctx, task)
	if err != nil {
		return Task{}, fmt.Errorf("UnblockTask: %w", err)
	}
	log.Printf("mwanachamataskmanager: UnblockTask: task %s blocked→awaiting-direction", taskID)
	return updated, nil
}
