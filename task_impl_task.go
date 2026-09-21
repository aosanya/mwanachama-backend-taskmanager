// task_impl_task.go — core Task CRUD implementation for [taskManager].
package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/gormstore"
)

// CreateTask creates a Task row.
//
// When task.WorkflowRunID is non-empty, the task is also linked to the named
// run via the started_task edge (denormalised WorkflowRunID column — see
// [TaskManager.AssignTask]'s chain-through behaviour). The edge write is
// best-effort: a failure is logged but does not roll back the task creation.
func (m *taskManager) CreateTask(ctx context.Context, task Task) (Task, error) {
	task.ID = "" // server-minted; a caller-supplied id is never honoured
	now := time.Now().UTC().Format(time.RFC3339)
	task.Status = TaskStatusPending
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.Priority == "" {
		task.Priority = TaskPriorityMedium
	}
	task.CompletedAt = ""

	row := gormstore.TaskToRow(task)
	if err := m.db.WithContext(ctx).Table(m.tables.Tasks).Create(&row).Error; err != nil {
		return Task{}, fmt.Errorf("CreateTask: %w", err)
	}

	out := gormstore.TaskFromRow(row)
	if task.WorkflowRunID != "" {
		if err := m.LinkTaskToRun(ctx, task.WorkflowRunID, out.ID); err != nil {
			log.Printf("mwanachamataskmanager: CreateTask: LinkTaskToRun run=%s task=%s: %v",
				task.WorkflowRunID, out.ID, err)
		}
		out.WorkflowRunID = task.WorkflowRunID
	}
	if len(task.Tags) > 0 {
		if err := m.setTaskTags(ctx, out.ID, task.Tags); err != nil {
			log.Printf("mwanachamataskmanager: CreateTask: setTaskTags task=%s: %v", out.ID, err)
		}
	}
	out.Tags = m.loadTagNames(ctx, out.ID)
	m.publish(ctx, TopicTaskCreated, TaskCreatedPayload{
		TaskID:        out.ID,
		Priority:      out.Priority,
		WorkflowRunID: out.WorkflowRunID,
	})
	return out, nil
}

// GetTask reads a single Task row.
func (m *taskManager) GetTask(ctx context.Context, taskID string) (Task, error) {
	var row gormstore.TaskRow
	err := m.db.WithContext(ctx).Table(m.tables.Tasks).
		Where("id = ? AND deleted = ?", taskID, false).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("GetTask: %w", err)
	}
	t := gormstore.TaskFromRow(row)
	t.Tags = m.loadTagNames(ctx, t.ID)
	return t, nil
}

// UpdateTask validates the requested status transition then patches the
// stored row.
//
// The pending → in_progress transition is additionally gated by the blocker
// rule: any inbound `blocks` edge whose source task has not reached a
// terminal status returns a *BlockedError listing the offending blocker task IDs.
func (m *taskManager) UpdateTask(ctx context.Context, task Task) (Task, error) {
	current, err := m.GetTask(ctx, task.ID)
	if err != nil {
		return Task{}, err
	}
	if current.Status != task.Status && !current.Status.CanTransitionTo(task.Status) {
		return Task{}, ErrInvalidStatusTransition
	}
	if current.Status == TaskStatusPending && task.Status == TaskStatusInProgress {
		if blockers, err := m.findActiveBlockers(ctx, task.ID); err != nil {
			return Task{}, err
		} else if len(blockers) > 0 {
			return Task{}, &BlockedError{BlockerTaskIDs: blockers}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	task.UpdatedAt = now
	task.CreatedAt = current.CreatedAt
	if isTerminalStatus(task.Status) && task.CompletedAt == "" {
		task.CompletedAt = now
	}
	task.AssignedTo = current.AssignedTo
	if task.WorkflowRunID == "" {
		task.WorkflowRunID = current.WorkflowRunID
	}

	row := gormstore.TaskToRow(task)
	if err := m.db.WithContext(ctx).Table(m.tables.Tasks).Where("id = ?", task.ID).
		Save(&row).Error; err != nil {
		return Task{}, fmt.Errorf("UpdateTask: %w", err)
	}
	if err := m.setTaskTags(ctx, task.ID, task.Tags); err != nil {
		log.Printf("mwanachamataskmanager: UpdateTask: setTaskTags task=%s: %v", task.ID, err)
	}

	out := gormstore.TaskFromRow(row)
	out.Tags = m.loadTagNames(ctx, out.ID)

	if out.WorkflowRunID != "" && current.WorkflowRunID == "" {
		if err := m.LinkTaskToRun(ctx, out.WorkflowRunID, out.ID); err != nil {
			slog.WarnContext(ctx, "UpdateTask: LinkTaskToRun", "run_id", out.WorkflowRunID, "task_id", out.ID, "err", err)
		}
	}

	if changed := nonStatusChangedFields(current, out); len(changed) > 0 {
		m.publish(ctx, TopicTaskUpdated, TaskUpdatedPayload{
			TaskID:        out.ID,
			ChangedFields: changed,
			WorkflowRunID: out.WorkflowRunID,
		})
	}
	if current.Status != out.Status {
		m.publish(ctx, TopicTaskStatusChanged, TaskStatusChangedPayload{
			TaskID:        out.ID,
			From:          current.Status,
			To:            out.Status,
			WorkflowRunID: out.WorkflowRunID,
		})
		if isTerminalStatus(out.Status) {
			completedAt := out.CompletedAt
			if completedAt == "" {
				completedAt = now
			}
			m.publish(ctx, TopicTaskCompleted, TaskCompletedPayload{
				TaskID:         out.ID,
				TerminalStatus: out.Status,
				CompletedAt:    completedAt,
				WorkflowRunID:  out.WorkflowRunID,
			})
		}
	}
	return out, nil
}

// DeleteTask soft-deletes the Task row.
func (m *taskManager) DeleteTask(ctx context.Context, taskID string) error {
	if _, err := m.GetTask(ctx, taskID); err != nil {
		return err
	}
	if err := m.db.WithContext(ctx).Table(m.tables.Tasks).Where("id = ?", taskID).
		UpdateColumn("deleted", true).Error; err != nil {
		return fmt.Errorf("DeleteTask: %w", err)
	}
	return nil
}

// ListTasks returns all non-deleted Task rows that match the filter.
func (m *taskManager) ListTasks(ctx context.Context, filter TaskFilter) ([]Task, error) {
	q := m.db.WithContext(ctx).Table(m.tables.Tasks).Where("deleted = ?", false)
	if filter.Status != "" {
		q = q.Where("status = ?", string(filter.Status))
	}
	if filter.Priority != "" {
		q = q.Where("priority = ?", string(filter.Priority))
	}
	if filter.WorkflowRunID != "" {
		q = q.Where("workflow_run_id = ?", filter.WorkflowRunID)
	}

	limit := filter.Limit
	if limit <= 0 || limit > maxListPage {
		limit = maxListPage
	}
	var rows []gormstore.TaskRow
	if err := q.Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("ListTasks: %w", err)
	}

	tasks := make([]Task, 0, len(rows))
	for _, r := range rows {
		t := gormstore.TaskFromRow(r)
		t.Tags = m.loadTagNames(ctx, t.ID)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// loadTagNames returns the names of every Tag linked to taskID via has_tag.
// Errors are silently swallowed — a missing or unreadable tag is omitted
// rather than failing the parent call.
func (m *taskManager) loadTagNames(ctx context.Context, taskID string) []string {
	var names []string
	err := m.db.WithContext(ctx).Table(m.tables.TaskTags).
		Joins("JOIN "+m.tables.Tags+" ON "+m.tables.Tags+".id = "+m.tables.TaskTags+".tag_id").
		Where(m.tables.TaskTags+".task_id = ?", taskID).
		Pluck(m.tables.Tags+".name", &names).Error
	if err != nil {
		return nil
	}
	return names
}

// setTaskTags replaces taskID's has_tag edges with one per name in tagNames,
// upserting each Tag by its unique Name.
func (m *taskManager) setTaskTags(ctx context.Context, taskID string, tagNames []string) error {
	if err := m.db.WithContext(ctx).Table(m.tables.TaskTags).Where("task_id = ?", taskID).Delete(nil).Error; err != nil {
		return fmt.Errorf("setTaskTags: clear: %w", err)
	}
	for _, name := range tagNames {
		if name == "" {
			continue
		}
		tagID, err := m.upsertTagByName(ctx, name)
		if err != nil {
			return fmt.Errorf("setTaskTags: upsert tag %q: %w", name, err)
		}
		if _, err := m.CreateRelationship(ctx, Relationship{Label: RelLabelHasTag, FromID: taskID, ToID: tagID}); err != nil {
			return fmt.Errorf("setTaskTags: link tag %q: %w", name, err)
		}
	}
	return nil
}

// upsertTagByName finds or creates a Tag row by its unique Name and returns
// its ID. A newly created row is minted a Code inside the same transaction
// as its insert; an existing row's Code is never touched.
func (m *taskManager) upsertTagByName(ctx context.Context, name string) (string, error) {
	var row gormstore.TagRow
	err := m.db.WithContext(ctx).Table(m.tables.Tags).Where("name = ?", name).First(&row).Error
	if err == nil {
		return row.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	txErr := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		code, err := gormstore.NextCode(ctx, tx, m.tables.CodeSequences, "tag", "TG")
		if err != nil {
			return err
		}
		row = gormstore.TagToRow(Tag{Name: name, Code: code, CreatedAt: now, UpdatedAt: now})
		return tx.Table(m.tables.Tags).Create(&row).Error
	})
	if txErr != nil {
		// Lost the race against a concurrent upsert of the same name — re-read.
		var existing gormstore.TagRow
		if reErr := m.db.WithContext(ctx).Table(m.tables.Tags).Where("name = ?", name).First(&existing).Error; reErr == nil {
			return existing.ID, nil
		}
		return "", txErr
	}
	return row.ID, nil
}

// publish emits an event via the optional Publisher. A nil publisher is
// silently skipped; errors are swallowed — events are best-effort and must
// not fail the originating operation.
func (m *taskManager) publish(ctx context.Context, topic string, payload any) {
	log.Printf("mwanachamataskmanager: publish: topic=%q payloadType=%T publisherNil=%v payload=%+v",
		topic, payload, m.publisher == nil, payload)
	if m.publisher == nil {
		return
	}
	_ = m.publisher.Publish(ctx, topic, payload)
}

// isTerminalStatus reports whether the status is one of the terminal
// lifecycle states (completed, failed, cancelled).
func isTerminalStatus(s TaskStatus) bool {
	switch s {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// nonStatusChangedFields lists the mutable Task property names that differ
// between before and after, excluding Status (reported separately via
// [TopicTaskStatusChanged]). Returns nil when nothing non-status differs.
func nonStatusChangedFields(before, after Task) []string {
	var out []string
	if before.Description != after.Description {
		out = append(out, "description")
	}
	if before.Priority != after.Priority {
		out = append(out, "priority")
	}
	if before.DueAt != after.DueAt {
		out = append(out, "due_at")
	}
	if !stringSlicesEqual(before.Tags, after.Tags) {
		out = append(out, "tags")
	}
	if before.EstimatedHours != after.EstimatedHours {
		out = append(out, "estimated_hours")
	}
	if before.Context != after.Context {
		out = append(out, "context")
	}
	return out
}

// stringSlicesEqual reports whether two string slices have identical
// length and elements in order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// findActiveBlockers returns the IDs of tasks that block taskID via inbound
// `blocks` edges and are themselves still non-terminal.
func (m *taskManager) findActiveBlockers(ctx context.Context, taskID string) ([]string, error) {
	edges, err := m.TraverseRelationships(ctx, taskID, RelLabelBlocks, DirectionInbound)
	if err != nil {
		return nil, fmt.Errorf("findActiveBlockers: %w", err)
	}
	var nonTerminal []string
	for _, e := range edges {
		blocker, err := m.GetTask(ctx, e.FromID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				continue
			}
			return nil, fmt.Errorf("findActiveBlockers: get %s: %w", e.FromID, err)
		}
		if !isTerminalStatus(blocker.Status) {
			nonTerminal = append(nonTerminal, blocker.ID)
		}
	}
	return nonTerminal, nil
}
