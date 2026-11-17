package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

type PendingTodosFilter struct {
	Backlog string `query:"backlog"`
	Limit   int    `query:"limit"`
	Offset  int    `query:"offset"`

	RequesterID string `query:"-" json:"-"`
}

type PendingTodosIssue struct {
	ID       string       `json:"id"`
	Code     string       `json:"code"`
	Title    string       `json:"title"`
	Priority TaskPriority `json:"priority"`
	Tags     []string     `json:"tags"`
	Step     string       `json:"step"`
	Todos    []TodoView   `json:"todos"`
	Done     int          `json:"done"`
	Total    int          `json:"total"`
}

type PendingTodos struct {
	Total             int                 `json:"total"`
	TotalPendingTodos int                 `json:"total_pending_todos"`
	Items             []PendingTodosIssue `json:"items"`
}

const inProgressStepName = "in progress"

var finishedTodoStatuses = []string{string(TodoStatusCompleted), string(TodoStatusSkipped)}

func (m *taskManager) PendingTodos(ctx context.Context, filter PendingTodosFilter) (PendingTodos, error) {
	out := PendingTodos{Items: []PendingTodosIssue{}}
	if strings.TrimSpace(filter.Backlog) == "" {
		return out, fmt.Errorf("PendingTodos: %w: backlog is required, send a backlog id or name", ErrInvalidFilter)
	}
	if filter.Offset < 0 {
		return out, fmt.Errorf("PendingTodos: %w: offset %d", ErrInvalidFilter, filter.Offset)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("PendingTodos: %w", err)
	}
	if err := m.requireVisibleBacklog(ctx, backlogs, strings.TrimSpace(filter.Backlog), filter.RequesterID); err != nil {
		return out, fmt.Errorf("PendingTodos: %w", err)
	}
	issues, err := m.inProgressIssues(ctx, filter)
	if err != nil {
		return out, fmt.Errorf("PendingTodos: %w", err)
	}
	var total int64
	if err := issues().Count(&total).Error; err != nil {
		return out, fmt.Errorf("PendingTodos: %w", err)
	}
	out.Total = int(total)
	var pending int64
	if err := m.store.Query(ctx, roleTaskTodo).
		Where("deleted = ? AND status NOT IN ? AND parent_task_id IN (?)", false, finishedTodoStatuses, issues().Select("id")).
		Count(&pending).Error; err != nil {
		return out, fmt.Errorf("PendingTodos: %w", err)
	}
	out.TotalPendingTodos = int(pending)
	var rows []pendingRow
	if err := issues().Select(pendingColumns).Order(taskOrder).
		Limit(pageLimit(filter.Limit)).Offset(filter.Offset).Scan(&rows).Error; err != nil {
		return out, fmt.Errorf("PendingTodos: %w", err)
	}
	if out.Items, err = m.issuesWithPendingTodos(ctx, backlogs, rows); err != nil {
		return out, fmt.Errorf("PendingTodos: %w", err)
	}
	return out, nil
}

func (m *taskManager) requireVisibleBacklog(ctx context.Context, backlogs []models.Backlog, name, requesterID string) error {
	found, ok := backlogNamed(backlogs, name)
	if ok {
		visible, err := m.backlogVisibility(ctx, requesterID)
		if err != nil {
			return err
		}
		ok = visible.Sees(found.ID)
	}
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownBacklog, name)
	}
	return nil
}

func (m *taskManager) inProgressIssues(ctx context.Context, filter PendingTodosFilter) (func() *gorm.DB, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(filter.Backlog)
	found, ok := backlogNamed(backlogs, name)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownBacklog, name)
	}
	if found.Smart() {
		where, args := atInProgressSteps(backlogs)
		return m.issuesAt(ctx, filter, name, "", where, args)
	}
	step, has := inProgressStep(found)
	if !has {
		return m.issuesAt(ctx, filter, name, "", "1 = 0", nil)
	}
	return m.issuesAt(ctx, filter, name, step.ID, "", nil)
}

func (m *taskManager) issuesAt(ctx context.Context, filter PendingTodosFilter, backlog, stepID, where string, args []any) (func() *gorm.DB, error) {
	if _, err := m.scopedTasks(ctx, backlog, stepID, nil, filter.RequesterID); err != nil {
		return nil, err
	}
	return func() *gorm.DB {
		q, _ := m.scopedTasks(ctx, backlog, stepID, nil, filter.RequesterID)
		if where != "" {
			q = q.Where(where, args...)
		}
		return q
	}, nil
}

func inProgressStep(b models.Backlog) (models.BacklogStep, bool) {
	for _, s := range b.Steps {
		if strings.EqualFold(strings.TrimSpace(s.Name), inProgressStepName) {
			return s, true
		}
	}
	return models.BacklogStep{}, false
}

func atInProgressSteps(backlogs []models.Backlog) (string, []any) {
	var pairs []string
	var args []any
	for _, b := range backlogs {
		if step, ok := inProgressStep(b); ok && !b.Smart() {
			pairs = append(pairs, "(backlog = ? AND step = ?)")
			args = append(args, b.ID, step.ID)
		}
	}
	if len(pairs) == 0 {
		return "1 = 0", nil
	}
	return "(" + strings.Join(pairs, " OR ") + ")", args
}

func (m *taskManager) issuesWithPendingTodos(ctx context.Context, backlogs []models.Backlog, rows []pendingRow) ([]PendingTodosIssue, error) {
	items := []PendingTodosIssue{}
	if len(rows) == 0 {
		return items, nil
	}
	ids := rowIDs(rows)
	counts, err := m.CountIssueTodos(ctx, ids)
	if err != nil {
		return nil, err
	}
	tags := m.tagNamesOf(ctx, ids)
	for _, r := range rows {
		todos, err := m.ListIssueTodos(ctx, r.ID)
		if err != nil {
			return nil, err
		}
		_, step := placeNames(backlogs, r)
		items = append(items, PendingTodosIssue{
			ID: r.ID, Code: r.Code, Title: r.Title, Priority: TaskPriority(r.Priority),
			Tags: tagsOrEmpty(tags[r.ID]), Step: step,
			Todos: unfinishedViews(viewsOfTodos(todos)),
			Done:  counts[r.ID].Done, Total: counts[r.ID].Total,
		})
	}
	return items, nil
}

func tagsOrEmpty(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}

func unfinishedViews(views []TodoView) []TodoView {
	kept := []TodoView{}
	for _, view := range views {
		if view.Status != TodoStatusCompleted && view.Status != TodoStatusSkipped {
			kept = append(kept, view)
		}
	}
	return kept
}
