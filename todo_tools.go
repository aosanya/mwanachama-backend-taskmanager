package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

var ErrInvalidTodoStatus = errors.New("a todo's status is To do, In progress, Done or Blocked")

type TodoView struct {
	TaskTodo
	State string `json:"state"`
}

type IssueTodoList struct {
	Items []TodoView `json:"items"`
	Done  int        `json:"done"`
	Total int        `json:"total"`
}

type IssueTodoCount struct {
	TaskID string `json:"task_id"`
	Done   int    `json:"done"`
	Total  int    `json:"total"`
}

type IssueTodoCounts struct {
	Items []IssueTodoCount `json:"items"`
}

type TodoCountsFilter struct {
	TaskIDs models.TagList `query:"task_id"`

	RequesterID string `query:"-" json:"-"`
}

type TodoChange struct {
	Title        *string   `json:"title"`
	Description  *string   `json:"description"`
	Instructions *string   `json:"instructions"`
	DependsOn    *[]string `json:"depends_on"`
}

type TodoOrder struct {
	IDs []string `json:"ids"`
}

func todoState(status TodoStatus) string {
	switch status {
	case TodoStatusPending:
		return "To do"
	case TodoStatusDispatched:
		return "In progress"
	case TodoStatusCompleted:
		return "Done"
	case TodoStatusBlocked, TodoStatusFailed:
		return "Blocked"
	}
	return string(status)
}

func todoStatusOf(word string) (TodoStatus, error) {
	switch strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(word, "_", " ")), " ")) {
	case "to do", "todo", "pending":
		return TodoStatusPending, nil
	case "in progress", "dispatched":
		return TodoStatusDispatched, nil
	case "done", "completed":
		return TodoStatusCompleted, nil
	case "blocked":
		return TodoStatusBlocked, nil
	}
	return "", fmt.Errorf("%w: got %q", ErrInvalidTodoStatus, word)
}

func settleTodos(todos []TaskTodo) []TaskTodo {
	live := map[string]bool{}
	finished := map[string]bool{}
	for _, todo := range todos {
		live[todo.ID] = true
		finished[todo.ID] = todo.Status == TodoStatusCompleted || todo.Status == TodoStatusSkipped
	}
	settled := make([]TaskTodo, len(todos))
	copy(settled, todos)
	for i, todo := range settled {
		heldByHand := todo.Status == TodoStatusBlocked && todo.BlockedReason != ""
		if heldByHand || (todo.Status != TodoStatusPending && todo.Status != TodoStatusBlocked) {
			continue
		}
		settled[i].Status = TodoStatusPending
		for _, dependency := range todo.DependsOn {
			if live[dependency] && !finished[dependency] {
				settled[i].Status = TodoStatusBlocked
			}
		}
	}
	return settled
}

func viewsOfTodos(todos []TaskTodo) []TodoView {
	views := make([]TodoView, 0, len(todos))
	for _, todo := range settleTodos(todos) {
		views = append(views, TodoView{TaskTodo: todo, State: todoState(todo.Status)})
	}
	return views
}

func (m *taskManager) viewOfTodo(ctx context.Context, todo TaskTodo) (TodoView, error) {
	siblings, err := m.ListIssueTodos(ctx, todo.ParentTaskID)
	if err != nil {
		return TodoView{}, err
	}
	for _, view := range viewsOfTodos(siblings) {
		if view.ID == todo.ID {
			return view, nil
		}
	}
	return TodoView{}, ErrTaskTodoNotFound
}

func (m *taskManager) issueTodoList(ctx context.Context, issueID string) (IssueTodoList, error) {
	todos, err := m.ListIssueTodos(ctx, issueID)
	if err != nil {
		return IssueTodoList{}, err
	}
	counts, err := m.CountIssueTodos(ctx, []string{issueID})
	if err != nil {
		return IssueTodoList{}, err
	}
	count := counts[issueID]
	return IssueTodoList{Items: viewsOfTodos(todos), Done: count.Done, Total: count.Total}, nil
}

func (m *taskManager) ListTodosOfIssue(ctx context.Context, requesterID, issueID string) (IssueTodoList, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, issueID); err != nil {
		return IssueTodoList{}, err
	}
	return m.issueTodoList(ctx, issueID)
}

func (m *taskManager) CountTodosOfIssues(ctx context.Context, filter TodoCountsFilter) (IssueTodoCounts, error) {
	out := IssueTodoCounts{Items: []IssueTodoCount{}}
	if len(filter.TaskIDs) == 0 {
		return out, fmt.Errorf("CountTodosOfIssues: %w: send task_id", ErrInvalidFilter)
	}
	if len(filter.TaskIDs) > maxListPage {
		return out, fmt.Errorf("CountTodosOfIssues: %w: at most %d task_id", ErrInvalidFilter, maxListPage)
	}
	q := m.store.Query(ctx, roleTask).Where("deleted = ? AND id IN ?", false, []string(filter.TaskIDs))
	q, err := m.narrowToVisible(ctx, q, filter.RequesterID)
	if err != nil {
		return out, fmt.Errorf("CountTodosOfIssues: %w", err)
	}
	var visible []string
	if err := q.Pluck("id", &visible).Error; err != nil {
		return out, fmt.Errorf("CountTodosOfIssues: %w", err)
	}
	if len(visible) == 0 {
		return out, nil
	}
	counts, err := m.CountIssueTodos(ctx, visible)
	if err != nil {
		return out, err
	}
	for _, id := range filter.TaskIDs {
		if count, ok := counts[id]; ok {
			out.Items = append(out.Items, IssueTodoCount{TaskID: id, Done: count.Done, Total: count.Total})
		}
	}
	return out, nil
}

func (m *taskManager) todoToEdit(ctx context.Context, requesterID, todoID string) (TaskTodo, error) {
	todo, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return TaskTodo{}, err
	}
	if _, err := m.GetTaskFor(ctx, requesterID, todo.ParentTaskID); err != nil {
		return TaskTodo{}, ErrTaskTodoNotFound
	}
	if err := m.requireMoveOnTask(ctx, requesterID, todo.ParentTaskID); err != nil {
		return TaskTodo{}, err
	}
	return todo, nil
}

func (m *taskManager) EditTodoFor(ctx context.Context, requesterID, todoID string, change TodoChange) (TodoView, error) {
	if _, err := m.todoToEdit(ctx, requesterID, todoID); err != nil {
		return TodoView{}, err
	}
	edited, err := m.UpdateTaskTodo(ctx, todoID, TaskTodoPatch{
		Title: change.Title, Description: change.Description,
		Instructions: change.Instructions, DependsOn: change.DependsOn,
	})
	if err != nil {
		return TodoView{}, err
	}
	return m.viewOfTodo(ctx, edited)
}

func (m *taskManager) ReorderTodosOfIssue(ctx context.Context, requesterID, issueID string, order TodoOrder) (IssueTodoList, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, issueID); err != nil {
		return IssueTodoList{}, err
	}
	if err := m.requireMoveOnTask(ctx, requesterID, issueID); err != nil {
		return IssueTodoList{}, err
	}
	if _, err := m.ReorderTaskTodos(ctx, issueID, order.IDs); err != nil {
		return IssueTodoList{}, err
	}
	return m.issueTodoList(ctx, issueID)
}

func (m *taskManager) DeleteTodoFor(ctx context.Context, requesterID, todoID string) error {
	if _, err := m.todoToEdit(ctx, requesterID, todoID); err != nil {
		return err
	}
	return m.DeleteTaskTodo(ctx, todoID)
}

func (m *taskManager) SetTodoStateFor(ctx context.Context, requesterID, todoID, state, blockedReason string) (TodoView, error) {
	status, err := todoStatusOf(state)
	if err != nil {
		return TodoView{}, err
	}
	blockedReason = strings.TrimSpace(blockedReason)
	before, err := m.todoToEdit(ctx, requesterID, todoID)
	if err != nil {
		return TodoView{}, err
	}
	set, err := m.SetTaskTodoStatus(ctx, todoID, status, blockedReason)
	if err != nil {
		return TodoView{}, err
	}
	m.moveToReviewWhenAllDone(ctx, requesterID, before, set)
	return m.viewOfTodo(ctx, set)
}
