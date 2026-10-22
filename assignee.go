package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func (m *taskManager) AssignTaskToHolder(ctx context.Context, taskID string, kind AssigneeKind, assigneeID string) error {
	if kind != models.AssigneeActor && kind != models.AssigneeRole {
		return ErrInvalidAssignee
	}
	if assigneeID == "" {
		return ErrInvalidAssignee
	}
	if _, err := m.GetTask(ctx, taskID); err != nil {
		return err
	}
	return m.handOver(ctx, taskID, kind, assigneeID)
}

func (m *taskManager) handOver(ctx context.Context, taskID string, kind AssigneeKind, assigneeID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := m.closeCurrentAssignment(ctx, taskID, now); err != nil {
		return err
	}
	if err := m.store.Query(ctx, roleTask).Where("id = ?", taskID).
		Updates(map[string]any{"assigned_to": assigneeID, "assignee_kind": string(kind)}).Error; err != nil {
		return fmt.Errorf("handOver: %w", err)
	}
	stretch := models.Assignment{
		ID:           newSpecID(),
		TaskID:       taskID,
		AssigneeKind: kind,
		AssigneeID:   assigneeID,
		AssignedAt:   now,
	}
	if err := m.store.Insert(ctx, roleAssignment, stretch); err != nil {
		return fmt.Errorf("handOver: record: %w", err)
	}
	return nil
}

func (m *taskManager) closeCurrentAssignment(ctx context.Context, taskID, at string) error {
	if err := m.store.Query(ctx, roleAssignment).
		Where("task_id = ? AND unassigned_at = ?", taskID, "").
		UpdateColumn("unassigned_at", at).Error; err != nil {
		return fmt.Errorf("closeCurrentAssignment: %w", err)
	}
	return nil
}

func (m *taskManager) ListTaskAssignments(ctx context.Context, taskID string) ([]Assignment, error) {
	if _, err := m.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	q := m.store.Query(ctx, roleAssignment).Where("task_id = ?", taskID).Order("assigned_at, id")
	out, err := specstore.List[Assignment](m.store, q, roleAssignment)
	if err != nil {
		return nil, fmt.Errorf("ListTaskAssignments: %w", err)
	}
	return out, nil
}

func (m *taskManager) ListHeldTasks(ctx context.Context, query HolderTasksQuery) ([]Task, error) {
	q := m.store.Query(ctx, roleTask).Where("deleted = ?", false)
	switch {
	case query.ActorID != "" && len(query.RoleKindIDs) > 0:
		q = q.Where("(assignee_kind = ? AND assigned_to = ?) OR (assignee_kind = ? AND assigned_to IN ?)",
			string(models.AssigneeActor), query.ActorID, string(models.AssigneeRole), query.RoleKindIDs)
	case query.ActorID != "":
		q = q.Where("assignee_kind = ? AND assigned_to = ?", string(models.AssigneeActor), query.ActorID)
	case len(query.RoleKindIDs) > 0:
		q = q.Where("assignee_kind = ? AND assigned_to IN ?", string(models.AssigneeRole), query.RoleKindIDs)
	default:
		return []Task{}, nil
	}
	tasks, err := specstore.List[Task](m.store, q.Order("created_at, id").Limit(maxListPage), roleTask)
	if err != nil {
		return nil, fmt.Errorf("ListHeldTasks: %w", err)
	}
	for i := range tasks {
		tasks[i].Tags = m.loadTagNames(ctx, tasks[i].ID)
	}
	return tasks, nil
}
