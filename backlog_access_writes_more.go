package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"
)

func (m *taskManager) requireMoveOnEveryTask(ctx context.Context, requesterID string, tasks []Task) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	for _, task := range tasks {
		if !visible.Moves(task.Backlog) {
			return fmt.Errorf("%w: this needs Can move on the backlog of every issue it touches", ErrBacklogForbidden)
		}
	}
	return nil
}

func (m *taskManager) requireMoveOnRun(ctx context.Context, requesterID, runID string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	tasks, err := m.ListTasksForRun(ctx, runID)
	if err != nil {
		return err
	}
	return m.requireMoveOnEveryTask(ctx, requesterID, tasks)
}

func (m *taskManager) requireMoveOnCase(ctx context.Context, requesterID, caseID string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	tasks, err := m.ListCaseTasks(ctx, caseID)
	if err != nil {
		return err
	}
	return m.requireMoveOnEveryTask(ctx, requesterID, tasks)
}

func (m *taskManager) AssignTaskAs(ctx context.Context, requesterID, taskID, agentID, workflowRunID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return err
	}
	return m.AssignTask(ctx, taskID, agentID, workflowRunID)
}

func (m *taskManager) UnassignTaskAs(ctx context.Context, requesterID, taskID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return err
	}
	return m.UnassignTask(ctx, taskID)
}

func (m *taskManager) AssignTaskToHolderAs(ctx context.Context, requesterID, taskID string, kind AssigneeKind, assigneeID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return err
	}
	return m.AssignTaskToHolder(ctx, taskID, kind, assigneeID)
}

func (m *taskManager) UnblockTaskAs(ctx context.Context, requesterID, taskID, note string) (Task, error) {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return Task{}, err
	}
	return m.UnblockTask(ctx, taskID, note)
}

func (m *taskManager) UnblockDependentsAs(ctx context.Context, requesterID, completedTaskID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, completedTaskID); err != nil {
		return err
	}
	return m.UnblockDependents(ctx, completedTaskID)
}

func (m *taskManager) AddTaskToProjectAs(ctx context.Context, requesterID, taskID, projectID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return err
	}
	return m.AddTaskToProject(ctx, taskID, projectID)
}

func (m *taskManager) RemoveTaskFromProjectAs(ctx context.Context, requesterID, taskID, projectID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return err
	}
	return m.RemoveTaskFromProject(ctx, taskID, projectID)
}

func (m *taskManager) CreateTaskInProjectAs(ctx context.Context, requesterID, projectName string, task Task) (Task, error) {
	target := task.Backlog
	if target == "" {
		first, err := m.firstBacklogID(ctx)
		if err != nil {
			return Task{}, err
		}
		target = first
	}
	if err := m.requireMoveOn(ctx, requesterID, target); err != nil {
		return Task{}, err
	}
	return m.CreateTaskInProject(ctx, projectName, task)
}

func (m *taskManager) LinkTaskToRunAs(ctx context.Context, requesterID, runID, taskID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return err
	}
	return m.LinkTaskToRun(ctx, runID, taskID)
}

func (m *taskManager) LinkCaseTaskAs(ctx context.Context, requesterID, caseID, taskID string) error {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return err
	}
	return m.LinkCaseTask(ctx, caseID, taskID)
}

func (m *taskManager) CancelWorkflowRunAs(ctx context.Context, requesterID, runID, reason, cancelledBy string, quiesceDeadline time.Time) (WorkflowRun, error) {
	if err := m.requireMoveOnRun(ctx, requesterID, runID); err != nil {
		return WorkflowRun{}, err
	}
	return m.CancelWorkflowRun(ctx, runID, reason, cancelledBy, quiesceDeadline)
}

func (m *taskManager) RollbackWorkflowRunAs(ctx context.Context, requesterID, runID, reason string) (WorkflowRun, error) {
	if err := m.requireMoveOnRun(ctx, requesterID, runID); err != nil {
		return WorkflowRun{}, err
	}
	return m.RollbackWorkflowRun(ctx, runID, reason)
}

func (m *taskManager) EndCaseAs(ctx context.Context, requesterID, caseID string, status CaseStatus, reply string) (Case, error) {
	if status == CaseStatusCancelled {
		if err := m.requireMoveOnCase(ctx, requesterID, caseID); err != nil {
			return Case{}, err
		}
	}
	return m.EndCase(ctx, caseID, status, reply)
}
