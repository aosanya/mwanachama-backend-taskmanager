package mwanachamataskmanager

import (
	"context"
	"fmt"
)

type AddTodosRequest struct {
	Todos []TodoSpec `json:"todos"`
}
type AddedTodo struct {
	ID         string `json:"id"`
	Code       string `json:"code"`
	Ordinality int    `json:"ordinality"`
}
type AddedTodos struct {
	Todos []AddedTodo `json:"todos"`
}

func (m *taskManager) AddTodos(ctx context.Context, requesterID, issueID string, req AddTodosRequest) (AddedTodos, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, issueID); err != nil {
		return AddedTodos{}, err
	}
	if err := m.requireMoveOnTask(ctx, requesterID, issueID); err != nil {
		return AddedTodos{}, err
	}
	if err := checkTodoSpecs(req.Todos); err != nil {
		return AddedTodos{}, fmt.Errorf("AddTodos: %w", err)
	}
	existing, err := m.ListIssueTodos(ctx, issueID)
	if err != nil {
		return AddedTodos{}, fmt.Errorf("AddTodos: %w", err)
	}
	offset := nextOrdinality(existing) - 1
	added := make([]AddedTodo, 0, len(req.Todos))
	for i, spec := range req.Todos {
		todo, err := m.CreateTaskTodo(ctx, TaskTodo{Title: spec.Title, Description: spec.Description, Instructions: spec.Instructions, CanRunParallel: spec.CanRunParallel, Ordinality: offset + i + 1, DependsOnOrdinalities: shiftOrdinalities(spec.WaitsOn, offset), ParentTaskID: issueID})
		if err != nil {
			m.undoAddTodos(ctx, added)
			return AddedTodos{}, fmt.Errorf("AddTodos: todo %d: %w", i+1, err)
		}
		added = append(added, AddedTodo{ID: todo.ID, Code: todo.Code, Ordinality: todo.Ordinality})
	}
	return AddedTodos{Todos: added}, nil
}
func (m *taskManager) undoAddTodos(ctx context.Context, added []AddedTodo) {
	for _, todo := range added {
		_ = m.DeleteTaskTodo(ctx, todo.ID)
	}
}
func shiftOrdinalities(positions []int, offset int) []int {
	if len(positions) == 0 {
		return nil
	}
	shifted := make([]int, len(positions))
	for i, position := range positions {
		shifted[i] = position + offset
	}
	return shifted
}
