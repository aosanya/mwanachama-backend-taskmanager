package mwanachamataskmanager

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	stuckDefaultStalledDays = 2
	stuckDefaultLimit       = 25
	stuckMaxLimit           = 100
)

type StuckFilter struct {
	Backlog     string         `query:"backlog"`
	Tag         models.TagList `query:"tag"`
	StalledDays int            `query:"stalled_days"`
	Reason      string         `query:"reason"`
	Limit       int            `query:"limit"`
	Offset      int            `query:"offset"`

	RequesterID string `query:"-" json:"-"`
}

type StuckItem struct {
	ID                string   `json:"id"`
	Code              string   `json:"code"`
	Title             string   `json:"title"`
	Assignee          string   `json:"assignee"`
	DaysInStep        int      `json:"days_in_step"`
	DaysSinceActivity int      `json:"days_since_activity"`
	TodosDone         int      `json:"todos_done"`
	TodosTotal        int      `json:"todos_total"`
	Reasons           []string `json:"reasons"`
	NextAction        string   `json:"next_action"`
}

type Stuck struct {
	Items []StuckItem `json:"items"`
	Total int         `json:"total"`
}

type stuckSubject struct {
	Task         Task
	Todos        []TaskTodo
	Others       []Task
	Now          time.Time
	LastActivity time.Time
	StalledAfter time.Duration
}

type stuckRule struct {
	Name       string
	NextAction string
	Fires      func(stuckSubject) bool
}

var stuckRules = []stuckRule{
	{"no_todos", "Write the issue's todos, or move it back to To do.", func(s stuckSubject) bool {
		return len(s.Todos) == 0
	}},
	{"no_todo_started", "Start the first todo, or release the issue back to To do.", func(s stuckSubject) bool {
		return len(s.Todos) > 0 && !anyTodo(s.Todos, func(t TaskTodo) bool {
			return t.Status == TodoStatusDispatched || t.Status == TodoStatusCompleted
		})
	}},
	{"all_remaining_todos_blocked", "Unblock a todo, or move the issue to Blocked with the reason.", func(s stuckSubject) bool {
		open := openTodos(s.Todos)
		if len(open) == 0 || anyTodo(open, func(t TaskTodo) bool { return t.Status == TodoStatusDispatched }) {
			return false
		}
		ready, _ := readyAndHeld(viewsOfTodos(s.Todos))
		return len(ready) == 0
	}},
	{"only_documentation_left_open", "Write the documentation todo, or move the issue to Review.", func(s stuckSubject) bool {
		open := openTodos(s.Todos)
		return len(open) > 0 &&
			!anyTodo(open, func(t TaskTodo) bool { return t.Status == TodoStatusDispatched }) &&
			!anyTodo(open, func(t TaskTodo) bool { return !mentionsAny(t.Title, []string{"document"}) })
	}},
	{"lease_expired", "Release the issue, or have the agent send a heartbeat.", func(s stuckSubject) bool {
		until, ok := stuckTime(s.Task.LeaseUntil)
		return ok && until.Before(s.Now)
	}},
	{"no_activity", "Ask the assignee for a note, or release the issue.", func(s stuckSubject) bool {
		return s.Now.Sub(s.LastActivity) >= s.StalledAfter
	}},
	{"unassigned_in_progress", "Assign the issue, or move it back to To do.", func(s stuckSubject) bool {
		return blank(s.Task.AssignedTo) && blank(s.Task.HeldBy)
	}},
	{"files_clash_with_another_in_progress", "Check task_file_overlaps and sequence the two issues.", func(s stuckSubject) bool {
		for _, other := range s.Others {
			if other.ID != s.Task.ID && len(overlappingPairs(s.Task.Files, other.Files)) > 0 {
				return true
			}
		}
		return false
	}},
}

func anyTodo(todos []TaskTodo, test func(TaskTodo) bool) bool {
	for _, todo := range todos {
		if test(todo) {
			return true
		}
	}
	return false
}

func openTodos(todos []TaskTodo) []TaskTodo {
	open := []TaskTodo{}
	for _, todo := range todos {
		if todo.Status != TodoStatusCompleted && todo.Status != TodoStatusSkipped {
			open = append(open, todo)
		}
	}
	return open
}

func stuckTime(text string) (time.Time, bool) {
	at, err := time.Parse(time.RFC3339, text)
	return at, err == nil
}

func stuckReasonKnown(name string) bool {
	for _, rule := range stuckRules {
		if rule.Name == name {
			return true
		}
	}
	return false
}

func stuckLimits(filter StuckFilter) (limit int, stalledAfter time.Duration, err error) {
	switch {
	case filter.Limit < 0 || filter.Limit > stuckMaxLimit:
		return 0, 0, fmt.Errorf("%w: limit %d is not between 1 and %d", ErrInvalidFilter, filter.Limit, stuckMaxLimit)
	case filter.Offset < 0:
		return 0, 0, fmt.Errorf("%w: offset %d is below 0", ErrInvalidFilter, filter.Offset)
	case filter.StalledDays < 0:
		return 0, 0, fmt.Errorf("%w: stalled_days %d is below 0", ErrInvalidFilter, filter.StalledDays)
	case filter.Reason != "" && !stuckReasonKnown(filter.Reason):
		return 0, 0, fmt.Errorf("%w: reason %q is not a stuck rule", ErrInvalidFilter, filter.Reason)
	}
	limit, days := filter.Limit, filter.StalledDays
	if limit == 0 {
		limit = stuckDefaultLimit
	}
	if days == 0 {
		days = stuckDefaultStalledDays
	}
	return limit, time.Duration(days) * oneDay, nil
}

func (m *taskManager) StuckIssues(ctx context.Context, filter StuckFilter) (Stuck, error) {
	out := Stuck{Items: []StuckItem{}}
	limit, stalledAfter, err := stuckLimits(filter)
	if err != nil {
		return out, fmt.Errorf("StuckIssues: %w", err)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("StuckIssues: %w", err)
	}
	if err := m.requireSeenBacklog(ctx, backlogs, NextReadyFilter{Backlog: filter.Backlog, RequesterID: filter.RequesterID}); err != nil {
		return out, fmt.Errorf("StuckIssues: %w", err)
	}
	narrowed := ReadyBoardFilter{Backlog: filter.Backlog, Tag: filter.Tag, RequesterID: filter.RequesterID}
	issues, err := m.readyBoardIssues(ctx, backlogs, narrowed)
	if err != nil {
		return out, fmt.Errorf("StuckIssues: %w", err)
	}
	issues = openIssues(issues)
	others := issues
	if filter.Backlog != "" || len(filter.Tag) > 0 {
		everyone, err := m.readyBoardIssues(ctx, backlogs, ReadyBoardFilter{RequesterID: filter.RequesterID})
		if err != nil {
			return out, fmt.Errorf("StuckIssues: %w", err)
		}
		others = openIssues(everyone)
	}
	ids := issueIDs(issues)
	todos, err := m.todosGroupedByIssue(ctx, ids)
	if err != nil {
		return out, fmt.Errorf("StuckIssues: %w", err)
	}
	noted, err := m.latestNoteTimes(ctx, ids)
	if err != nil {
		return out, fmt.Errorf("StuckIssues: %w", err)
	}
	now := time.Now().UTC()
	stuck := []StuckItem{}
	for _, issue := range issues {
		subject := stuckSubject{Task: issue, Todos: todos[issue.ID], Others: others, Now: now, StalledAfter: stalledAfter}
		subject.LastActivity = lastActivityOf(subject, noted[issue.ID])
		if item, found := stuckItemOf(subject, filter.Reason); found {
			stuck = append(stuck, item)
		}
	}
	sort.SliceStable(stuck, func(i, j int) bool {
		if len(stuck[i].Reasons) != len(stuck[j].Reasons) {
			return len(stuck[i].Reasons) > len(stuck[j].Reasons)
		}
		return stuck[i].DaysSinceActivity > stuck[j].DaysSinceActivity
	})
	out.Total = len(stuck)
	if filter.Offset < len(stuck) {
		out.Items = stuck[filter.Offset:min(filter.Offset+limit, len(stuck))]
	}
	return out, nil
}

func stuckItemOf(subject stuckSubject, onlyReason string) (StuckItem, bool) {
	reasons := []string{}
	nextAction := ""
	for _, rule := range stuckRules {
		if rule.Fires(subject) {
			reasons = append(reasons, rule.Name)
			if nextAction == "" {
				nextAction = rule.NextAction
			}
		}
	}
	if len(reasons) == 0 || (onlyReason != "" && !containsString(reasons, onlyReason)) {
		return StuckItem{}, false
	}
	issue := subject.Task
	assignee := issue.AssignedTo
	if assignee == "" {
		assignee = issue.HeldBy
	}
	_, daysInStep := stepClock(issue, subject.Now)
	return StuckItem{
		ID: issue.ID, Code: issue.Code, Title: issue.Title, Assignee: assignee,
		DaysInStep:        daysInStep,
		DaysSinceActivity: int(subject.Now.Sub(subject.LastActivity) / oneDay),
		TodosDone:         countTodos(subject.Todos, TodoStatusCompleted),
		TodosTotal:        len(subject.Todos),
		Reasons:           reasons,
		NextAction:        nextAction,
	}, true
}

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func countTodos(todos []TaskTodo, status TodoStatus) int {
	count := 0
	for _, todo := range todos {
		if todo.Status == status {
			count++
		}
	}
	return count
}

func lastActivityOf(subject stuckSubject, noted time.Time) time.Time {
	latest := noted
	consider := func(text string) {
		if at, ok := stuckTime(text); ok && at.After(latest) {
			latest = at
		}
	}
	consider(subject.Task.UpdatedAt)
	if latest.IsZero() {
		consider(subject.Task.CreatedAt)
	}
	for _, todo := range subject.Todos {
		consider(todo.UpdatedAt)
	}
	return latest
}

func openIssues(issues []Task) []Task {
	open := make([]Task, 0, len(issues))
	for _, issue := range issues {
		if !containsString(closedStatuses, string(issue.Status)) {
			open = append(open, issue)
		}
	}
	return open
}

func (m *taskManager) latestNoteTimes(ctx context.Context, ids []string) (map[string]time.Time, error) {
	latest := map[string]time.Time{}
	for start := 0; start < len(ids); start += readyBoardIssuesPerQuery {
		end := min(start+readyBoardIssuesPerQuery, len(ids))
		var rows []struct {
			TaskID string
			Latest string
		}
		err := m.store.Query(ctx, roleTaskNote).
			Select("task_id, MAX(created_at) AS latest").
			Where("task_id IN ?", ids[start:end]).
			Group("task_id").
			Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if at, ok := stuckTime(strings.TrimSpace(row.Latest)); ok {
				latest[row.TaskID] = at
			}
		}
	}
	return latest, nil
}
