package mwanachamataskmanager

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	snapshotDefaultStalledDays = 2
	snapshotColumns            = "id, code, title, status, backlog, step, assigned_to, updated_at"
	oneDay                     = 24 * time.Hour
)

type BoardSnapshotFilter struct {
	Backlog     string `query:"backlog"`
	StalledDays int    `query:"stalled_days"`

	RequesterID string `query:"-" json:"-"`
}

type SnapshotStep struct {
	Name       string `json:"name"`
	Count      int    `json:"count"`
	OldestDays int    `json:"oldest_days"`
	OldestCode string `json:"oldest_code,omitempty"`
}

type SnapshotAgent struct {
	Agent      string   `json:"agent"`
	InProgress int      `json:"in_progress"`
	Codes      []string `json:"codes"`
}

type SnapshotStalled struct {
	ID    string `json:"id"`
	Code  string `json:"code"`
	Title string `json:"title"`
	Agent string `json:"agent"`
	Days  int    `json:"days"`
}

type SnapshotWait struct {
	Code    string `json:"code"`
	Blocked bool   `json:"blocked"`
}

type SnapshotBlocked struct {
	ID      string         `json:"id"`
	Code    string         `json:"code"`
	Title   string         `json:"title"`
	WaitsOn []SnapshotWait `json:"waits_on"`
}

type SnapshotTodoProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type BoardSnapshot struct {
	Steps         []SnapshotStep       `json:"steps"`
	Agents        []SnapshotAgent      `json:"agents"`
	Stalled       []SnapshotStalled    `json:"stalled"`
	BlockedChains []SnapshotBlocked    `json:"blocked_chains"`
	Ready         int                  `json:"ready"`
	TodoProgress  SnapshotTodoProgress `json:"todo_progress"`
}

type snapshotRow struct {
	ID         string
	Code       string
	Title      string
	Status     string
	Backlog    string
	Step       string
	AssignedTo string
	UpdatedAt  string
}

func (r snapshotRow) place() pendingRow {
	return pendingRow{ID: r.ID, Code: r.Code, Title: r.Title, Status: r.Status, Backlog: r.Backlog, Step: r.Step}
}

type snapshotIssue struct {
	row      snapshotRow
	stepName string
	age      time.Duration
	aged     bool
	tags     []string
}

func (m *taskManager) BoardSnapshot(ctx context.Context, filter BoardSnapshotFilter) (BoardSnapshot, error) {
	if filter.StalledDays < 0 {
		return BoardSnapshot{}, fmt.Errorf("BoardSnapshot: %w: stalled_days %d", ErrInvalidFilter, filter.StalledDays)
	}
	stalledAfter := time.Duration(snapshotDefaultStalledDays) * oneDay
	if filter.StalledDays > 0 {
		stalledAfter = time.Duration(filter.StalledDays) * oneDay
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return BoardSnapshot{}, fmt.Errorf("BoardSnapshot: %w", err)
	}
	backlog := strings.TrimSpace(filter.Backlog)
	if err := m.requireSeenBacklog(ctx, backlogs, NextReadyFilter{Backlog: backlog, RequesterID: filter.RequesterID}); err != nil {
		return BoardSnapshot{}, fmt.Errorf("BoardSnapshot: %w", err)
	}
	query, err := m.pendingBase(ctx, PendingFilter{Backlog: backlog, RequesterID: filter.RequesterID}, false)
	if err != nil {
		return BoardSnapshot{}, fmt.Errorf("BoardSnapshot: %w", err)
	}
	var rows []snapshotRow
	if err := query.Select(snapshotColumns).Order(taskOrder).Scan(&rows).Error; err != nil {
		return BoardSnapshot{}, fmt.Errorf("BoardSnapshot: %w", err)
	}
	marks, err := m.markedIssueTags(ctx)
	if err != nil {
		return BoardSnapshot{}, fmt.Errorf("BoardSnapshot: %w", err)
	}
	issues := snapshotIssues(backlogs, rows, marks, time.Now().UTC())
	out := BoardSnapshot{
		Steps:         snapshotSteps(backlogs, backlog, issues),
		Agents:        []SnapshotAgent{},
		Stalled:       []SnapshotStalled{},
		BlockedChains: []SnapshotBlocked{},
	}
	inProgress := []snapshotIssue{}
	for _, issue := range issues {
		if strings.EqualFold(issue.stepName, inProgressStepName) {
			inProgress = append(inProgress, issue)
		}
		if isToDoStep(backlogs, issue.row.place()) && markOfTags(issue.tags) == tagReady {
			out.Ready++
		}
	}
	out.Agents = snapshotAgents(inProgress)
	out.Stalled = snapshotStalled(inProgress, stalledAfter)
	if out.BlockedChains, err = m.snapshotBlocked(ctx, issues); err != nil {
		return BoardSnapshot{}, fmt.Errorf("BoardSnapshot: %w", err)
	}
	if out.TodoProgress, err = m.snapshotTodoProgress(ctx, inProgress); err != nil {
		return BoardSnapshot{}, fmt.Errorf("BoardSnapshot: %w", err)
	}
	return out, nil
}

func snapshotIssues(backlogs []models.Backlog, rows []snapshotRow, marks map[string][]string, now time.Time) []snapshotIssue {
	issues := make([]snapshotIssue, len(rows))
	for i, row := range rows {
		_, step := placeNames(backlogs, row.place())
		issues[i] = snapshotIssue{row: row, stepName: step, tags: marks[row.ID]}
		if touched, err := time.Parse(time.RFC3339, row.UpdatedAt); err == nil {
			issues[i].age, issues[i].aged = max(now.Sub(touched), 0), true
		}
	}
	return issues
}

func snapshotSteps(backlogs []models.Backlog, backlogName string, issues []snapshotIssue) []SnapshotStep {
	steps := map[string]*SnapshotStep{}
	oldest := map[string]time.Duration{}
	for _, issue := range issues {
		step, seen := steps[issue.stepName]
		if !seen {
			step = &SnapshotStep{Name: issue.stepName}
			steps[issue.stepName] = step
		}
		step.Count++
		if issue.aged && issue.age >= oldest[issue.stepName] {
			oldest[issue.stepName] = issue.age
			step.OldestDays, step.OldestCode = int(issue.age/oneDay), issue.row.Code
		}
	}
	out := []SnapshotStep{}
	listed := map[string]bool{}
	list := func(name string, always bool) {
		if listed[name] {
			return
		}
		if step, ok := steps[name]; ok {
			out = append(out, *step)
			listed[name] = true
		} else if always {
			out = append(out, SnapshotStep{Name: name})
			listed[name] = true
		}
	}
	chosen, onlyChosen := backlogNamed(backlogs, backlogName)
	onlyChosen = onlyChosen && backlogName != "" && !chosen.Smart()
	for _, b := range backlogs {
		if onlyChosen && b.ID != chosen.ID {
			continue
		}
		for _, s := range b.Steps {
			list(s.Name, onlyChosen)
		}
	}
	list(noStepName, false)
	return out
}

func snapshotAgents(inProgress []snapshotIssue) []SnapshotAgent {
	byAgent := map[string]*SnapshotAgent{}
	for _, issue := range inProgress {
		agent, seen := byAgent[issue.row.AssignedTo]
		if !seen {
			agent = &SnapshotAgent{Agent: issue.row.AssignedTo, Codes: []string{}}
			byAgent[issue.row.AssignedTo] = agent
		}
		agent.InProgress++
		agent.Codes = append(agent.Codes, issue.row.Code)
	}
	out := make([]SnapshotAgent, 0, len(byAgent))
	for _, agent := range byAgent {
		out = append(out, *agent)
	}
	slices.SortFunc(out, func(a, b SnapshotAgent) int {
		if a.InProgress != b.InProgress {
			return b.InProgress - a.InProgress
		}
		return strings.Compare(a.Agent, b.Agent)
	})
	return out
}

func snapshotStalled(inProgress []snapshotIssue, after time.Duration) []SnapshotStalled {
	out := []SnapshotStalled{}
	for _, issue := range inProgress {
		if issue.aged && issue.age > after {
			out = append(out, SnapshotStalled{
				ID: issue.row.ID, Code: issue.row.Code, Title: issue.row.Title,
				Agent: issue.row.AssignedTo, Days: int(issue.age / oneDay),
			})
		}
	}
	slices.SortStableFunc(out, func(a, b SnapshotStalled) int { return b.Days - a.Days })
	return out
}

func (m *taskManager) snapshotBlocked(ctx context.Context, issues []snapshotIssue) ([]SnapshotBlocked, error) {
	out := []SnapshotBlocked{}
	byID := make(map[string]snapshotIssue, len(issues))
	blockedIDs := []string{}
	for _, issue := range issues {
		byID[issue.row.ID] = issue
		if isBlocked(issue.tags) {
			blockedIDs = append(blockedIDs, issue.row.ID)
		}
	}
	if len(blockedIDs) == 0 {
		return out, nil
	}
	var edges []struct{ FromTaskID, ToTaskID string }
	if err := m.store.Query(ctx, roleDependency).Select("from_task_id, to_task_id").Where("from_task_id IN ?", blockedIDs).Scan(&edges).Error; err != nil {
		return nil, err
	}
	waits := map[string][]SnapshotWait{}
	for _, edge := range edges {
		if target, open := byID[edge.ToTaskID]; open {
			waits[edge.FromTaskID] = append(waits[edge.FromTaskID], SnapshotWait{Code: target.row.Code, Blocked: isBlocked(target.tags)})
		}
	}
	for _, id := range blockedIDs {
		blocked := byID[id].row
		list := waits[id]
		slices.SortFunc(list, func(a, b SnapshotWait) int { return strings.Compare(a.Code, b.Code) })
		out = append(out, SnapshotBlocked{ID: id, Code: blocked.Code, Title: blocked.Title, WaitsOn: append([]SnapshotWait{}, list...)})
	}
	return out, nil
}

func (m *taskManager) snapshotTodoProgress(ctx context.Context, inProgress []snapshotIssue) (SnapshotTodoProgress, error) {
	ids := make([]string, len(inProgress))
	for i, issue := range inProgress {
		ids[i] = issue.row.ID
	}
	counts, err := m.CountIssueTodos(ctx, ids)
	if err != nil {
		return SnapshotTodoProgress{}, err
	}
	var progress SnapshotTodoProgress
	for _, count := range counts {
		progress.Done += count.Done
		progress.Total += count.Total
	}
	return progress, nil
}

func (m *taskManager) markedIssueTags(ctx context.Context) (map[string][]string, error) {
	out := map[string][]string{}
	lowered := make([]string, len(markTags))
	for i, name := range markTags {
		lowered[i] = strings.ToLower(name)
	}
	var tags []struct{ ID, Name string }
	if err := m.store.Query(ctx, roleTag).Select("id, name").Where("LOWER(name) IN ?", lowered).Scan(&tags).Error; err != nil || len(tags) == 0 {
		return out, err
	}
	names := make(map[string]string, len(tags))
	tagIDs := make([]string, len(tags))
	for i, t := range tags {
		names[t.ID], tagIDs[i] = t.Name, t.ID
	}
	var links []struct{ TaskID, TagID string }
	if err := m.store.Query(ctx, roleTagging).Select("task_id, tag_id").Where("tag_id IN ?", tagIDs).Scan(&links).Error; err != nil {
		return nil, err
	}
	for _, l := range links {
		out[l.TaskID] = append(out[l.TaskID], names[l.TagID])
	}
	return out, nil
}
