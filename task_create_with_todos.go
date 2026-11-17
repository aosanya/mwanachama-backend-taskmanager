package mwanachamataskmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type TodoSpec struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	Instructions   string `json:"instructions"`
	CanRunParallel bool   `json:"can_run_parallel"`
	WaitsOn        []int  `json:"waits_on"`
}

type CreateWithTodosRequest struct {
	Task
	Todos []TodoSpec `json:"todos"`
}

func (r *CreateWithTodosRequest) UnmarshalJSON(b []byte) error {
	type flat CreateWithTodosRequest
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return err
	}
	nestedKey := nestedTaskKey(keys)
	if nestedKey == "" {
		return json.Unmarshal(b, (*flat)(r))
	}
	for key := range keys {
		if key != nestedKey && key != "todos" {
			return fmt.Errorf("%w: send the issue fields at the top level or under %s, not both", ErrInvalidTask, nestedKey)
		}
	}
	if err := json.Unmarshal(keys[nestedKey], &r.Task); err != nil {
		return err
	}
	if raw, ok := keys["todos"]; ok {
		return json.Unmarshal(raw, &r.Todos)
	}
	return nil
}

func nestedTaskKey(keys map[string]json.RawMessage) string {
	for _, key := range []string{"Task", "task"} {
		if _, ok := keys[key]; ok {
			return key
		}
	}
	return ""
}

type IssueWithTodos struct {
	Issue Task       `json:"issue"`
	Todos []TodoView `json:"todos"`
}

func (m *taskManager) CreateWithTodos(ctx context.Context, requesterID string, req CreateWithTodosRequest) (IssueWithTodos, error) {
	if strings.TrimSpace(req.Title) == "" {
		return IssueWithTodos{}, fmt.Errorf("CreateWithTodos: %w: an issue needs a title", ErrInvalidTask)
	}
	if err := checkTodoSpecs(req.Todos); err != nil {
		return IssueWithTodos{}, fmt.Errorf("CreateWithTodos: %w", err)
	}
	issue, err := m.CreateTaskAs(ctx, requesterID, req.Task)
	if err != nil {
		return IssueWithTodos{}, err
	}
	var made []string
	for i, spec := range req.Todos {
		todo, err := m.CreateTaskTodo(ctx, TaskTodo{
			Title:                 spec.Title,
			Description:           spec.Description,
			Instructions:          spec.Instructions,
			CanRunParallel:        spec.CanRunParallel,
			Ordinality:            i + 1,
			DependsOnOrdinalities: spec.WaitsOn,
			ParentTaskID:          issue.ID,
		})
		if err != nil {
			m.undoCreateWithTodos(ctx, issue.ID, made)
			return IssueWithTodos{}, fmt.Errorf("CreateWithTodos: todo %d: %w", i+1, err)
		}
		made = append(made, todo.ID)
	}
	list, err := m.issueTodoList(ctx, issue.ID)
	if err != nil {
		return IssueWithTodos{}, err
	}
	issue, err = m.GetTask(ctx, issue.ID)
	if err != nil {
		return IssueWithTodos{}, err
	}
	return IssueWithTodos{Issue: issue, Todos: list.Items}, nil
}

func (m *taskManager) undoCreateWithTodos(ctx context.Context, issueID string, todoIDs []string) {
	for _, id := range todoIDs {
		_ = m.DeleteTaskTodo(ctx, id)
	}
	_ = m.DeleteTask(ctx, issueID)
}

func checkTodoSpecs(specs []TodoSpec) error {
	if len(specs) == 0 {
		return fmt.Errorf("%w: send at least one todo", ErrInvalidTask)
	}
	for i, spec := range specs {
		position := i + 1
		if strings.TrimSpace(spec.Title) == "" || strings.TrimSpace(spec.Instructions) == "" {
			return fmt.Errorf("%w: todo %d needs a title and instructions", ErrInvalidTask, position)
		}
		for _, waitsOn := range spec.WaitsOn {
			if waitsOn < 1 || waitsOn >= position {
				return fmt.Errorf("%w: todo %d waits on todo %d, which is not an earlier todo", ErrInvalidTask, position, waitsOn)
			}
		}
	}
	return nil
}
