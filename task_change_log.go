package mwanachamataskmanager

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	changeLabelMoved    = "Moved"
	changeLabelAssignee = "Assignee"
	changeLabelStatus   = "Status"
	changeLabelTags     = "Tags"
	changeTextDeleted   = "Deleted"
	changeNone          = "none"
)

func (m *taskManager) logTaskChanges(ctx context.Context, before, after Task) {
	if from, to := placeText(before), placeText(after); from != to {
		m.logChange(ctx, before.ID, changeLabelMoved, from, to)
	}
	if before.Status != after.Status {
		m.logChange(ctx, before.ID, changeLabelStatus, string(before.Status), string(after.Status))
	}
	if from, to := tagsText(before.Tags), tagsText(after.Tags); from != to {
		m.logChange(ctx, before.ID, changeLabelTags, from, to)
	}
	if before.AssignedTo != after.AssignedTo {
		m.logChange(ctx, before.ID, changeLabelAssignee, before.AssignedTo, after.AssignedTo)
	}
}

func (m *taskManager) logMove(ctx context.Context, task Task, path []TaskStatus, backlogID, stepID string) {
	after := task
	after.Backlog, after.Step = backlogID, stepID
	if len(path) > 0 {
		after.Status = path[len(path)-1]
	}
	m.logTaskChanges(ctx, task, after)
}

func (m *taskManager) logAssignee(ctx context.Context, before Task, assigneeID string) {
	after := before
	after.AssignedTo = assigneeID
	m.logTaskChanges(ctx, before, after)
}

func (m *taskManager) logStatusSet(ctx context.Context, before Task, status TaskStatus, place map[string]any) {
	after := before
	after.Status = status
	if backlogID, ok := place["backlog"].(string); ok {
		after.Backlog = backlogID
	}
	if stepID, ok := place["step"].(string); ok {
		after.Step = stepID
	}
	m.logTaskChanges(ctx, before, after)
}

func (m *taskManager) logClaim(ctx context.Context, task Task, target models.BacklogStep, hold leaseHold) {
	after := task
	after.Step = target.ID
	after.Status = TaskStatusInProgress
	if hold.Agent != "" {
		after.AssignedTo = hold.Agent
	}
	m.logTaskChanges(ctx, task, after)
}

func (m *taskManager) logChange(ctx context.Context, taskID, label, from, to string) {
	m.writeHubNote(ctx, taskID, noteAutomatic, fmt.Sprintf("%s %s to %s", label, orNone(from), orNone(to)))
}

func (m *taskManager) logDeletion(ctx context.Context, taskID string) {
	m.writeHubNote(ctx, taskID, noteAutomatic, changeTextDeleted)
}

func placeText(task Task) string {
	if task.Backlog == "" && task.Step == "" {
		return ""
	}
	return task.Backlog + "/" + task.Step
}

func tagsText(tags []string) string {
	sorted := slices.Clone(tags)
	slices.Sort(sorted)
	return strings.Join(sorted, ",")
}

func orNone(value string) string {
	if value == "" {
		return changeNone
	}
	return value
}
