package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

// CreateTaskTodo creates a TaskTodo row. Todos with non-empty DependsOn
// start as [TodoStatusBlocked] and are NOT dispatched — the caller must
// call [DispatchTaskTodo] once their predecessors complete. Todos with no
// dependencies start as [TodoStatusPending]; callers should call
// [DispatchTaskTodo] immediately after creation.
//
// A Code is minted for the new row inside the same transaction as its
// insert.
func (m *taskManager) CreateTaskTodo(ctx context.Context, todo TaskTodo) (TaskTodo, error) {
	todo.ID = newSpecID()
	if todo.Title == "" || todo.Instructions == "" || todo.ParentTaskID == "" {
		return TaskTodo{}, fmt.Errorf("%w: title, instructions, and parent_task_id are required", ErrInvalidTask)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if len(todo.DependsOn) > 0 {
		todo.Status = TodoStatusBlocked
	} else {
		todo.Status = TodoStatusPending
	}
	todo.CreatedAt = now
	todo.UpdatedAt = now

	txErr := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		code, err := m.nextCode(ctx, tx, "task_todo", "TD")
		if err != nil {
			return err
		}
		todo.Code = code
		row, err := encode(m.store.Object(roleTaskTodo), todo)
		if err != nil {
			return err
		}
		return tx.Table(m.store.Table(roleTaskTodo)).Create(row).Error
	})
	if txErr != nil {
		return TaskTodo{}, fmt.Errorf("CreateTaskTodo: %w", txErr)
	}
	return todo, nil
}

// DispatchTaskTodo publishes [TopicTodoDispatched] for an existing todo so
// agents can pick it up. If the todo is currently [TodoStatusBlocked], it
// is first advanced to [TodoStatusPending].
func (m *taskManager) DispatchTaskTodo(ctx context.Context, todoID string) error {
	todo, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return err
	}
	if todo.Status == TodoStatusBlocked {
		now := time.Now().UTC().Format(time.RFC3339)
		if err := m.store.Query(ctx, roleTaskTodo).Where("id = ?", todoID).
			Updates(map[string]any{"status": string(TodoStatusPending), "updated_at": now}).Error; err != nil {
			return fmt.Errorf("DispatchTaskTodo: unblock %s: %w", todoID, err)
		}
		todo.Status = TodoStatusPending
		todo.UpdatedAt = now
	}
	m.publish(ctx, TopicTodoDispatched, TodoDispatchedPayload{
		TodoID:         todo.ID,
		TaskID:         todo.ParentTaskID,
		ParentTaskID:   todo.ParentTaskID,
		DecompRunID:    todo.DecompRunID,
		AgentID:        todo.AgentID,
		Title:          todo.Title,
		Instructions:   todo.Instructions,
		Ordinality:     todo.Ordinality,
		CanRunParallel: todo.CanRunParallel,
		DependsOn:      todo.DependsOn,
		Precalls:       todo.Precalls,
		WorkflowRunID:  todo.WorkflowRunID,
	})
	return nil
}

// GetTaskTodo reads a single TaskTodo row.
func (m *taskManager) GetTaskTodo(ctx context.Context, todoID string) (TaskTodo, error) {
	var todo TaskTodo
	q := m.store.Query(ctx, roleTaskTodo).Where("id = ? AND deleted = ?", todoID, false)
	if err := m.store.Take(q, roleTaskTodo, &todo, ErrTaskTodoNotFound); err != nil {
		if errors.Is(err, ErrTaskTodoNotFound) {
			return TaskTodo{}, err
		}
		return TaskTodo{}, fmt.Errorf("GetTaskTodo: %w", err)
	}
	return todo, nil
}

// UpdateTaskTodoStatus transitions a TaskTodo to a new [TodoStatus].
func (m *taskManager) UpdateTaskTodoStatus(ctx context.Context, todoID string, status TodoStatus) (TaskTodo, error) {
	if _, err := m.GetTaskTodo(ctx, todoID); err != nil {
		return TaskTodo{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res := m.store.Query(ctx, roleTaskTodo).Where("id = ? AND deleted = ?", todoID, false).
		Updates(map[string]any{"status": string(status), "updated_at": now})
	if res.Error != nil {
		return TaskTodo{}, fmt.Errorf("UpdateTaskTodoStatus: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return TaskTodo{}, ErrTaskTodoNotFound
	}
	return m.GetTaskTodo(ctx, todoID)
}

// ListTaskTodos returns all non-deleted TaskTodos, optionally filtered by
// workflowRunID. When workflowRunID is empty, all todos are returned.
func (m *taskManager) ListTaskTodos(ctx context.Context, workflowRunID string) ([]TaskTodo, error) {
	q := m.store.Query(ctx, roleTaskTodo).Where("deleted = ?", false)
	if workflowRunID != "" {
		q = q.Where("workflow_run_id = ?", workflowRunID)
	}
	out, err := specstore.List[TaskTodo](m.store, q.Limit(maxListPage), roleTaskTodo)
	if err != nil {
		return nil, fmt.Errorf("ListTaskTodos: %w", err)
	}
	return out, nil
}
