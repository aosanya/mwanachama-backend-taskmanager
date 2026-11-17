package mwanachamataskmanager

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	readyBoardDefaultLimit   = 25
	readyBoardMaxLimit       = 100
	readyBoardIssuesPerQuery = 100
)

type ReadyBoardFilter struct {
	Backlog string         `query:"backlog"`
	Tag     models.TagList `query:"tag"`
	Limit   int            `query:"limit"`

	RequesterID string `query:"-" json:"-"`
}

type ReadyBoardTodo struct {
	ReadyTodo
	IssueID    string   `json:"issue_id"`
	IssueCode  string   `json:"issue_code"`
	IssueFiles []string `json:"issue_files"`
}

type ReadyBoardTodos struct {
	Items []ReadyBoardTodo `json:"items"`
	Total int              `json:"total"`
}

func (m *taskManager) ReadyTodosOfBoard(ctx context.Context, filter ReadyBoardFilter) (ReadyBoardTodos, error) {
	out := ReadyBoardTodos{Items: []ReadyBoardTodo{}}
	limit, err := readyBoardLimit(filter.Limit)
	if err != nil {
		return out, fmt.Errorf("ReadyTodosOfBoard: %w", err)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("ReadyTodosOfBoard: %w", err)
	}
	if err := m.requireSeenBacklog(ctx, backlogs, NextReadyFilter{Backlog: filter.Backlog, RequesterID: filter.RequesterID}); err != nil {
		return out, fmt.Errorf("ReadyTodosOfBoard: %w", err)
	}
	issues, err := m.readyBoardIssues(ctx, backlogs, filter)
	if err != nil {
		return out, fmt.Errorf("ReadyTodosOfBoard: %w", err)
	}
	todos, err := m.todosGroupedByIssue(ctx, issueIDs(issues))
	if err != nil {
		return out, fmt.Errorf("ReadyTodosOfBoard: %w", err)
	}
	for _, issue := range issues {
		ready, _ := readyAndHeld(viewsOfTodos(todos[issue.ID]))
		for _, todo := range ready {
			out.Total++
			if len(out.Items) < limit {
				out.Items = append(out.Items, ReadyBoardTodo{
					ReadyTodo: todo, IssueID: issue.ID, IssueCode: issue.Code, IssueFiles: filesOrEmpty(issue.Files),
				})
			}
		}
	}
	return out, nil
}

func readyBoardLimit(asked int) (int, error) {
	switch {
	case asked < 0 || asked > readyBoardMaxLimit:
		return 0, fmt.Errorf("%w: limit %d is not between 1 and %d", ErrInvalidFilter, asked, readyBoardMaxLimit)
	case asked == 0:
		return readyBoardDefaultLimit, nil
	}
	return asked, nil
}

func (m *taskManager) readyBoardIssues(ctx context.Context, backlogs []models.Backlog, filter ReadyBoardFilter) ([]Task, error) {
	q, err := m.scopedTasks(ctx, filter.Backlog, "", filter.Tag, filter.RequesterID)
	if err != nil {
		return nil, err
	}
	where, args := atInProgressSteps(backlogs)
	return specstore.List[Task](m.store, q.Where(where, args...).Order(taskOrder).Limit(maxListPage), roleTask)
}

func (m *taskManager) todosGroupedByIssue(ctx context.Context, ids []string) (map[string][]TaskTodo, error) {
	byIssue := map[string][]TaskTodo{}
	for start := 0; start < len(ids); start += readyBoardIssuesPerQuery {
		end := min(start+readyBoardIssuesPerQuery, len(ids))
		q := m.store.Query(ctx, roleTaskTodo).
			Where("parent_task_id IN ? AND deleted = ?", ids[start:end], false).
			Order("ordinality, created_at, id")
		rows, err := specstore.List[TaskTodo](m.store, q, roleTaskTodo)
		if err != nil {
			return nil, err
		}
		for _, todo := range rows {
			byIssue[todo.ParentTaskID] = append(byIssue[todo.ParentTaskID], todo)
		}
	}
	for id, todos := range byIssue {
		byIssue[id] = withElapsedAll(todos)
	}
	return byIssue, nil
}

func issueIDs(issues []Task) []string {
	ids := make([]string, 0, len(issues))
	for _, issue := range issues {
		ids = append(ids, issue.ID)
	}
	return ids
}

func filesOrEmpty(files FileList) []string {
	if files == nil {
		return []string{}
	}
	return files
}
