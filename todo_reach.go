package mwanachamataskmanager

import "context"

func (m *taskManager) todoReached(ctx context.Context, scope readScope, todo TaskTodo, cache map[string]bool) bool {
	if scope.open() {
		return true
	}
	if todo.ParentTaskID == "" {
		return false
	}
	if seen, ok := cache[todo.ParentTaskID]; ok {
		return seen
	}
	issue, err := m.GetTask(ctx, todo.ParentTaskID)
	reached := err == nil && scope.sees(issue)
	cache[todo.ParentTaskID] = reached
	return reached
}

func (m *taskManager) requireTodoReach(ctx context.Context, requesterID, todoID string) (TaskTodo, error) {
	todo, err := m.GetTaskTodo(ctx, todoID)
	if err != nil {
		return TaskTodo{}, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil {
		return TaskTodo{}, err
	}
	if !m.todoReached(ctx, scope, todo, map[string]bool{}) {
		return TaskTodo{}, ErrTaskTodoNotFound
	}
	return todo, nil
}

func (m *taskManager) GetTaskTodoFor(ctx context.Context, requesterID, todoID string) (TaskTodo, error) {
	return m.requireTodoReach(ctx, requesterID, todoID)
}

func (m *taskManager) ListTaskTodosFor(ctx context.Context, requesterID, workflowRunID string) ([]TaskTodo, error) {
	todos, err := m.ListTaskTodos(ctx, workflowRunID)
	if err != nil {
		return nil, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil {
		return nil, err
	}
	cache := map[string]bool{}
	kept := make([]TaskTodo, 0, len(todos))
	for _, todo := range todos {
		if m.todoReached(ctx, scope, todo, cache) {
			kept = append(kept, todo)
		}
	}
	return kept, nil
}

func (m *taskManager) CreateTaskTodoAs(ctx context.Context, requesterID string, todo TaskTodo) (TaskTodo, error) {
	if todo.ParentTaskID != "" {
		if _, err := m.GetTaskFor(ctx, requesterID, todo.ParentTaskID); err != nil {
			return TaskTodo{}, err
		}
		if err := m.requireMoveOnTask(ctx, requesterID, todo.ParentTaskID); err != nil {
			return TaskTodo{}, err
		}
	} else if requesterID != "" {
		return TaskTodo{}, ErrTaskNotFound
	}
	return m.CreateTaskTodo(ctx, todo)
}

func (m *taskManager) DispatchTaskTodoAs(ctx context.Context, requesterID, todoID string) error {
	todo, err := m.requireTodoReach(ctx, requesterID, todoID)
	if err != nil {
		return err
	}
	if err := m.requireMoveOnTask(ctx, requesterID, todo.ParentTaskID); err != nil {
		return err
	}
	return m.DispatchTaskTodo(ctx, todoID)
}

func (m *taskManager) LinkTodoToRunAs(ctx context.Context, requesterID, runID, todoID string) error {
	todo, err := m.requireTodoReach(ctx, requesterID, todoID)
	if err != nil {
		return err
	}
	if err := m.requireMoveOnTask(ctx, requesterID, todo.ParentTaskID); err != nil {
		return err
	}
	return m.LinkTodoToRun(ctx, runID, todoID)
}

func (m *taskManager) ListProjectsForTaskFor(ctx context.Context, requesterID, taskID string) ([]Project, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return nil, err
	}
	return m.ListProjectsForTask(ctx, taskID)
}

func (m *taskManager) ListDeliverablesForTaskFor(ctx context.Context, requesterID, taskID string) ([]Deliverable, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return nil, err
	}
	return m.ListDeliverablesForTask(ctx, taskID)
}

func (m *taskManager) ListAcceptanceCriteriaForTaskFor(ctx context.Context, requesterID, taskID string) ([]AcceptanceCriteria, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return nil, err
	}
	return m.ListAcceptanceCriteriaForTask(ctx, taskID)
}

func (m *taskManager) GetWorkflowRunClosureFor(ctx context.Context, requesterID, runID string) (WorkflowRunClosure, error) {
	closure, err := m.GetWorkflowRunClosure(ctx, runID)
	if err != nil {
		return WorkflowRunClosure{}, err
	}
	scope, err := m.readScope(ctx, requesterID)
	if err != nil || scope.open() {
		return closure, err
	}
	hidden := map[string]bool{}
	closure.Tasks = keepReached(closure.Tasks, scope, hidden)
	cache := map[string]bool{}
	todos := make([]TaskTodo, 0, len(closure.Todos))
	for _, todo := range closure.Todos {
		if m.todoReached(ctx, scope, todo, cache) {
			todos = append(todos, todo)
			continue
		}
		hidden[todo.ID] = true
	}
	closure.Todos = todos
	edges := make([]Relationship, 0, len(closure.Edges))
	for _, e := range closure.Edges {
		if !hidden[e.FromID] && !hidden[e.ToID] {
			edges = append(edges, e)
		}
	}
	closure.Edges = edges
	return closure, nil
}

func keepReached(tasks []Task, scope readScope, hidden map[string]bool) []Task {
	kept := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		if scope.sees(t) {
			kept = append(kept, t)
			continue
		}
		hidden[t.ID] = true
	}
	return kept
}
