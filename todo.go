package mwanachamataskmanager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339) }

func dependsOnCell(ids []string) any {
	if len(ids) == 0 {
		return nil
	}
	raw, _ := json.Marshal(ids)
	return string(raw)
}

func uniqueIDs(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (m *taskManager) CreateTaskTodo(ctx context.Context, todo TaskTodo) (TaskTodo, error) {
	todo.ID = newSpecID()
	if todo.Title == "" || todo.Instructions == "" || todo.ParentTaskID == "" {
		return TaskTodo{}, fmt.Errorf("%w: title, instructions, and parent_task_id are required", ErrInvalidTask)
	}
	siblings, err := m.ListIssueTodos(ctx, todo.ParentTaskID)
	if err != nil {
		return TaskTodo{}, err
	}
	dependsOn, err := m.resolveDependencies(ctx, todo.ParentTaskID, siblings, todo.DependsOn, todo.DependsOnOrdinalities)
	if err != nil {
		return TaskTodo{}, err
	}
	todo.DependsOn = dependsOn
	todo.DependsOnOrdinalities = nil
	if todo.Ordinality <= 0 {
		todo.Ordinality = nextOrdinality(siblings)
	}
	todo.BlockedReason = ""
	now := nowStamp()
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
		row, err := m.store.Row(roleTaskTodo, todo)
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

func nextOrdinality(todos []TaskTodo) int {
	highest := 0
	for _, todo := range todos {
		if todo.Ordinality > highest {
			highest = todo.Ordinality
		}
	}
	return highest + 1
}

func (m *taskManager) resolveDependencies(ctx context.Context, issueID string, siblings []TaskTodo, ids []string, ordinalities []int) ([]string, error) {
	byOrdinality := make(map[int]string, len(siblings))
	for _, sibling := range siblings {
		byOrdinality[sibling.Ordinality] = sibling.ID
	}
	resolved := append([]string(nil), ids...)
	for _, ordinality := range ordinalities {
		id, ok := byOrdinality[ordinality]
		if !ok {
			return nil, fmt.Errorf("%w: no todo %d in this issue", ErrTaskTodoDependencyNotFound, ordinality)
		}
		resolved = append(resolved, id)
	}
	resolved = uniqueIDs(resolved)
	if err := m.CheckTodoSameIssue(ctx, issueID, resolved); err != nil {
		return nil, err
	}
	return resolved, nil
}

func (m *taskManager) DispatchTaskTodo(ctx context.Context, todoID string) error {
	todo, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return err
	}
	if todo.Status == TodoStatusBlocked {
		now := nowStamp()
		if err := m.store.Query(ctx, roleTaskTodo).Where("id = ?", todoID).
			Updates(map[string]any{"status": string(TodoStatusPending), "blocked_reason": "", "updated_at": now}).Error; err != nil {
			return fmt.Errorf("DispatchTaskTodo: unblock %s: %w", todoID, err)
		}
		todo.Status = TodoStatusPending
		todo.BlockedReason = ""
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

func (m *taskManager) GetTaskTodo(ctx context.Context, todoID string) (TaskTodo, error) {
	var todo TaskTodo
	q := m.store.Query(ctx, roleTaskTodo).Where("id = ? AND deleted = ?", todoID, false)
	if err := m.store.Take(q, roleTaskTodo, &todo, ErrTaskTodoNotFound); err != nil {
		if errors.Is(err, ErrTaskTodoNotFound) {
			return TaskTodo{}, err
		}
		return TaskTodo{}, fmt.Errorf("GetTaskTodo: %w", err)
	}
	return withElapsed(todo, time.Now().UTC()), nil
}

func (m *taskManager) UpdateTaskTodoStatus(ctx context.Context, todoID string, status TodoStatus) (TaskTodo, error) {
	return m.writeTodoStatus(ctx, "UpdateTaskTodoStatus", todoID, status, "")
}

func (m *taskManager) SetTaskTodoStatus(ctx context.Context, todoID string, status TodoStatus, blockedReason string) (TaskTodo, error) {
	if status == TodoStatusBlocked && blockedReason == "" {
		return TaskTodo{}, ErrTaskTodoBlockedReasonRequired
	}
	if status != TodoStatusBlocked {
		blockedReason = ""
	}
	return m.writeTodoStatus(ctx, "SetTaskTodoStatus", todoID, status, blockedReason)
}

func (m *taskManager) writeTodoStatus(ctx context.Context, caller, todoID string, status TodoStatus, blockedReason string) (TaskTodo, error) {
	before, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return TaskTodo{}, err
	}
	now := nowStamp()
	changes := timesForStatus(before, status, now)
	changes["status"] = string(status)
	changes["blocked_reason"] = blockedReason
	changes["updated_at"] = now
	res := m.store.Query(ctx, roleTaskTodo).Where("id = ? AND deleted = ?", todoID, false).Updates(changes)
	if res.Error != nil {
		return TaskTodo{}, fmt.Errorf("%s: %w", caller, res.Error)
	}
	if res.RowsAffected == 0 {
		return TaskTodo{}, ErrTaskTodoNotFound
	}
	return m.GetTaskTodo(ctx, todoID)
}

func (m *taskManager) ListTaskTodos(ctx context.Context, workflowRunID string) ([]TaskTodo, error) {
	q := m.store.Query(ctx, roleTaskTodo).Where("deleted = ?", false)
	if workflowRunID != "" {
		q = q.Where("workflow_run_id = ?", workflowRunID)
	}
	out, err := specstore.List[TaskTodo](m.store, q.Limit(maxListPage), roleTaskTodo)
	if err != nil {
		return nil, fmt.Errorf("ListTaskTodos: %w", err)
	}
	return withElapsedAll(out), nil
}

func (m *taskManager) ListIssueTodos(ctx context.Context, issueID string) ([]TaskTodo, error) {
	if issueID == "" {
		return nil, fmt.Errorf("%w: an issue id is required", ErrInvalidTask)
	}
	q := m.store.Query(ctx, roleTaskTodo).
		Where("parent_task_id = ? AND deleted = ?", issueID, false).
		Order("ordinality, created_at, id").Limit(maxListPage)
	out, err := specstore.List[TaskTodo](m.store, q, roleTaskTodo)
	if err != nil {
		return nil, fmt.Errorf("ListIssueTodos: %w", err)
	}
	return withElapsedAll(out), nil
}

func (m *taskManager) CountIssueTodos(ctx context.Context, issueIDs []string) (map[string]TodoCount, error) {
	counts := map[string]TodoCount{}
	issueIDs = uniqueIDs(issueIDs)
	if len(issueIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		ParentTaskID string
		Total        int
		Done         int
	}
	err := m.store.Query(ctx, roleTaskTodo).
		Select("parent_task_id, count(*) as total, coalesce(sum(case when status = ? then 1 else 0 end), 0) as done", string(TodoStatusCompleted)).
		Where("parent_task_id IN ? AND deleted = ?", issueIDs, false).
		Group("parent_task_id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("CountIssueTodos: %w", err)
	}
	for _, row := range rows {
		counts[row.ParentTaskID] = TodoCount{Done: row.Done, Total: row.Total}
	}
	return counts, nil
}

func (m *taskManager) UpdateTaskTodo(ctx context.Context, todoID string, patch TaskTodoPatch) (TaskTodo, error) {
	todo, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return TaskTodo{}, err
	}
	changes := map[string]any{}
	if patch.Title != nil {
		if *patch.Title == "" {
			return TaskTodo{}, fmt.Errorf("%w: a todo needs a title", ErrInvalidTask)
		}
		changes["title"] = *patch.Title
	}
	if patch.Instructions != nil {
		if *patch.Instructions == "" {
			return TaskTodo{}, fmt.Errorf("%w: a todo needs instructions", ErrInvalidTask)
		}
		changes["instructions"] = *patch.Instructions
	}
	if patch.Description != nil {
		changes["description"] = *patch.Description
	}
	if patch.DependsOn != nil {
		dependsOn := uniqueIDs(*patch.DependsOn)
		if err := m.CheckTodoSameIssue(ctx, todo.ParentTaskID, dependsOn); err != nil {
			return TaskTodo{}, err
		}
		if err := m.CheckTodoCycle(ctx, todoID, dependsOn); err != nil {
			return TaskTodo{}, err
		}
		changes["depends_on"] = dependsOnCell(dependsOn)
		changes["can_run_parallel"] = len(dependsOn) == 0
	}
	if len(changes) == 0 {
		return todo, nil
	}
	changes["updated_at"] = nowStamp()
	if err := m.store.Query(ctx, roleTaskTodo).Where("id = ? AND deleted = ?", todoID, false).Updates(changes).Error; err != nil {
		return TaskTodo{}, fmt.Errorf("UpdateTaskTodo: %w", err)
	}
	return m.GetTaskTodo(ctx, todoID)
}

func (m *taskManager) ReorderTaskTodos(ctx context.Context, issueID string, orderedIDs []string) ([]TaskTodo, error) {
	live, err := m.ListIssueTodos(ctx, issueID)
	if err != nil {
		return nil, err
	}
	if len(orderedIDs) != len(live) || len(uniqueIDs(orderedIDs)) != len(live) {
		return nil, ErrTaskTodoOrderMismatch
	}
	held := make(map[string]bool, len(live))
	for _, todo := range live {
		held[todo.ID] = true
	}
	for _, id := range orderedIDs {
		if !held[id] {
			return nil, ErrTaskTodoOrderMismatch
		}
	}
	now := nowStamp()
	txErr := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for at, id := range orderedIDs {
			if err := m.within(tx, roleTaskTodo).Where("id = ?", id).
				Updates(map[string]any{"ordinality": at + 1, "updated_at": now}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		return nil, fmt.Errorf("ReorderTaskTodos: %w", txErr)
	}
	return m.ListIssueTodos(ctx, issueID)
}

func (m *taskManager) DeleteTaskTodo(ctx context.Context, todoID string) error {
	todo, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return err
	}
	siblings, err := m.ListIssueTodos(ctx, todo.ParentTaskID)
	if err != nil {
		return err
	}
	now := nowStamp()
	txErr := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, sibling := range siblings {
			kept := withoutID(sibling.DependsOn, todoID)
			if len(kept) == len(sibling.DependsOn) {
				continue
			}
			changes := map[string]any{"depends_on": dependsOnCell(kept), "updated_at": now}
			if len(kept) == 0 {
				changes["can_run_parallel"] = true
			}
			if err := m.within(tx, roleTaskTodo).Where("id = ?", sibling.ID).Updates(changes).Error; err != nil {
				return err
			}
		}
		return m.within(tx, roleTaskTodo).Where("id = ?", todoID).
			Updates(map[string]any{"deleted": true, "updated_at": now}).Error
	})
	if txErr != nil {
		return fmt.Errorf("DeleteTaskTodo: %w", txErr)
	}
	return nil
}

func withoutID(ids []string, drop string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != drop {
			out = append(out, id)
		}
	}
	return out
}

func (m *taskManager) CheckTodoSameIssue(ctx context.Context, issueID string, dependsOn []string) error {
	for _, id := range dependsOn {
		other, err := m.GetTaskTodo(ctx, id)
		if errors.Is(err, ErrTaskTodoNotFound) {
			return fmt.Errorf("%w: %s", ErrTaskTodoDependencyNotFound, id)
		}
		if err != nil {
			return err
		}
		if other.ParentTaskID != issueID {
			return fmt.Errorf("%w: %s belongs to another issue", ErrTaskTodoCrossIssue, id)
		}
	}
	return nil
}

func (m *taskManager) CheckTodoCycle(ctx context.Context, todoID string, dependsOn []string) error {
	todo, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return err
	}
	siblings, err := m.ListIssueTodos(ctx, todo.ParentTaskID)
	if err != nil {
		return err
	}
	waitsOn := make(map[string][]string, len(siblings))
	for _, sibling := range siblings {
		waitsOn[sibling.ID] = sibling.DependsOn
	}
	waitsOn[todoID] = dependsOn

	reaches := map[string]bool{}
	var leadsBack func(id string) bool
	leadsBack = func(id string) bool {
		if id == todoID {
			return true
		}
		if seen, ok := reaches[id]; ok {
			return seen
		}
		reaches[id] = false
		for _, next := range waitsOn[id] {
			if leadsBack(next) {
				reaches[id] = true
				return true
			}
		}
		return false
	}
	for _, id := range dependsOn {
		if leadsBack(id) {
			return fmt.Errorf("%w: %s would wait on %s", ErrTaskTodoCycle, todo.Code, id)
		}
	}
	return nil
}

type legacyTodoDependencies struct {
	ID           string
	ParentTaskID string
	Ordinality   int
	Deleted      bool
	DependsOn    string
}

func (m *taskManager) migrateTodoDependencies(ctx context.Context) error {
	table := m.store.Table(roleTaskTodo)
	if !m.db.Migrator().HasTable(table) {
		return nil
	}
	var holding []legacyTodoDependencies
	err := m.store.Query(ctx, roleTaskTodo).
		Select("id, parent_task_id, ordinality, deleted, cast(depends_on as text) as depends_on").
		Where("depends_on IS NOT NULL AND cast(depends_on as text) NOT LIKE ?", "%\"%").
		Scan(&holding).Error
	if err != nil {
		return fmt.Errorf("migrate todo dependencies: %w", err)
	}
	if len(holding) == 0 {
		return nil
	}
	parents := map[string]bool{}
	for _, row := range holding {
		parents[row.ParentTaskID] = true
	}
	idsByOrdinality, err := m.idsByOrdinality(ctx, parents)
	if err != nil {
		return err
	}
	for _, row := range holding {
		var numbers []int
		if err := json.Unmarshal([]byte(row.DependsOn), &numbers); err != nil {
			continue
		}
		ids := make([]string, 0, len(numbers))
		for _, number := range numbers {
			if id, ok := idsByOrdinality[row.ParentTaskID][number]; ok && id != row.ID {
				ids = append(ids, id)
			}
		}
		err := m.store.Query(ctx, roleTaskTodo).Where("id = ?", row.ID).
			Update("depends_on", dependsOnCell(uniqueIDs(ids))).Error
		if err != nil {
			return fmt.Errorf("migrate todo dependencies of %s: %w", row.ID, err)
		}
	}
	return nil
}

func (m *taskManager) idsByOrdinality(ctx context.Context, parents map[string]bool) (map[string]map[int]string, error) {
	issueIDs := make([]string, 0, len(parents))
	for id := range parents {
		issueIDs = append(issueIDs, id)
	}
	var rows []legacyTodoDependencies
	err := m.store.Query(ctx, roleTaskTodo).
		Select("id, parent_task_id, ordinality, deleted").
		Where("parent_task_id IN ?", issueIDs).
		Order("deleted desc, created_at, id").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("migrate todo dependencies: %w", err)
	}
	out := map[string]map[int]string{}
	for _, row := range rows {
		if out[row.ParentTaskID] == nil {
			out[row.ParentTaskID] = map[int]string{}
		}
		out[row.ParentTaskID][row.Ordinality] = row.ID
	}
	return out, nil
}
