package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

const (
	lintDefaultLimit = 50
	lintMaxLimit     = 200
)

var doneMarkers = []string{"done looks like", "pass when", "done when"}

type LintFilter struct {
	TaskID  string `query:"taskID"`
	Backlog string `query:"backlog"`
	Step    string `query:"step"`
	Limit   int    `query:"limit,min=1"`
	Offset  int    `query:"offset,min=0"`

	RequesterID string `query:"-" json:"-"`
}

type LintFinding struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type LintItem struct {
	ID       string        `json:"id"`
	Code     string        `json:"code"`
	Title    string        `json:"title"`
	Findings []LintFinding `json:"findings"`
}

type Lint struct {
	Items []LintItem `json:"items"`
	Clean int        `json:"clean"`
	Total int        `json:"total"`
}

type lintSubject struct {
	Task  Task
	Todos []TaskTodo
}

type lintRule struct {
	Name    string
	Message string
	Broken  func(lintSubject) bool
}

var lintRules = []lintRule{
	{"no_description", "The issue has no description.", func(s lintSubject) bool {
		return blank(s.Task.Description)
	}},
	{"no_done_when", "The description does not say when the issue is done (Done looks like, Pass when or Done when).", func(s lintSubject) bool {
		return !blank(s.Task.Description) && !mentionsAny(s.Task.Description, doneMarkers)
	}},
	{"no_todos", "The issue has no todos.", func(s lintSubject) bool {
		return len(s.Todos) == 0
	}},
	{"last_todo_not_documentation", "The last todo is not a documentation todo.", func(s lintSubject) bool {
		return len(s.Todos) > 0 && !mentionsAny(s.Todos[len(s.Todos)-1].Title, []string{"document"})
	}},
	{"no_files", "The issue declares no files, so overlaps with other issues cannot be checked.", func(s lintSubject) bool {
		return len(s.Task.Files) == 0
	}},
	{"effort_unknown", "The effort is unknown.", func(s lintSubject) bool {
		return s.Task.Effort == "" || s.Task.Effort == TaskEffortUnknown
	}},
	{"no_priority", "The issue has no priority.", func(s lintSubject) bool {
		return s.Task.Priority == ""
	}},
	{"todo_without_description", "At least one todo has no description.", func(s lintSubject) bool {
		for _, todo := range s.Todos {
			if blank(todo.Description) {
				return true
			}
		}
		return false
	}},
}

func blank(text string) bool {
	return strings.TrimSpace(text) == ""
}

func mentionsAny(text string, needles []string) bool {
	lowered := strings.ToLower(text)
	for _, needle := range needles {
		if strings.Contains(lowered, needle) {
			return true
		}
	}
	return false
}

func findingsOf(subject lintSubject) []LintFinding {
	findings := []LintFinding{}
	for _, rule := range lintRules {
		if rule.Broken(subject) {
			findings = append(findings, LintFinding{Rule: rule.Name, Message: rule.Message})
		}
	}
	return findings
}

func (m *taskManager) Lint(ctx context.Context, filter LintFilter) (Lint, error) {
	out := Lint{Items: []LintItem{}}
	if filter.Limit > lintMaxLimit {
		return out, fmt.Errorf("Lint: %w: limit %d is above %d", ErrInvalidFilter, filter.Limit, lintMaxLimit)
	}
	tasks, total, err := m.lintedIssues(ctx, filter)
	if err != nil {
		return out, fmt.Errorf("Lint: %w", err)
	}
	todos, err := m.todosOfIssues(ctx, tasks)
	if err != nil {
		return out, fmt.Errorf("Lint: %w", err)
	}
	out.Total = total
	for _, task := range tasks {
		findings := findingsOf(lintSubject{Task: task, Todos: todos[task.ID]})
		if len(findings) == 0 {
			out.Clean++
		}
		out.Items = append(out.Items, LintItem{ID: task.ID, Code: task.Code, Title: task.Title, Findings: findings})
	}
	return out, nil
}

func (m *taskManager) lintedIssues(ctx context.Context, filter LintFilter) ([]Task, int, error) {
	if id := strings.TrimSpace(filter.TaskID); id != "" {
		task, err := m.GetTaskFor(ctx, filter.RequesterID, id)
		if err != nil {
			return nil, 0, err
		}
		return []Task{task}, 1, nil
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return nil, 0, err
	}
	if err := m.requireSeenBacklog(ctx, backlogs, NextReadyFilter{Backlog: filter.Backlog, RequesterID: filter.RequesterID}); err != nil {
		return nil, 0, err
	}
	open := func() (*gorm.DB, error) {
		q, err := m.scopedTasks(ctx, strings.TrimSpace(filter.Backlog), strings.TrimSpace(filter.Step), nil, filter.RequesterID)
		if err != nil {
			return nil, err
		}
		return q.Where("status NOT IN ?", closedStatuses), nil
	}
	counting, err := open()
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := counting.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	listing, err := open()
	if err != nil {
		return nil, 0, err
	}
	limit := filter.Limit
	if limit == 0 {
		limit = lintDefaultLimit
	}
	tasks, err := specstore.List[Task](m.store, listing.Order(taskOrder).Limit(limit).Offset(filter.Offset), roleTask)
	return tasks, int(total), err
}

func (m *taskManager) todosOfIssues(ctx context.Context, tasks []Task) (map[string][]TaskTodo, error) {
	byIssue := map[string][]TaskTodo{}
	if len(tasks) == 0 {
		return byIssue, nil
	}
	ids := make([]string, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	q := m.store.Query(ctx, roleTaskTodo).
		Where("parent_task_id IN ? AND deleted = ?", ids, false).
		Order("parent_task_id, ordinality, created_at, id")
	todos, err := specstore.List[TaskTodo](m.store, q, roleTaskTodo)
	if err != nil {
		return nil, err
	}
	for _, todo := range todos {
		byIssue[todo.ParentTaskID] = append(byIssue[todo.ParentTaskID], todo)
	}
	return byIssue, nil
}
