package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	nextReadyDefaultLimit = 1
	nextReadyMaxLimit     = 5
	toDoStepName          = "To do"
)

type NextReadyFilter struct {
	Tag     models.TagList `query:"tag"`
	Backlog string         `query:"backlog"`
	Limit   int            `query:"limit,min=1"`

	Priority     models.TaskPriority `query:"priority"`
	Repo         string              `query:"repo"`
	Assignee     string              `query:"assignee"`
	Unassigned   bool                `query:"unassigned"`
	HasTodos     *bool               `query:"has_todos"`
	TodosOpen    bool                `query:"todos_open"`
	Effort       string              `query:"effort"`
	CreatedSince string              `query:"created_since"`
	UpdatedSince string              `query:"updated_since"`
	HasFiles     *bool               `query:"has_files"`
	Fields       []string            `query:"fields"`

	RequesterID string `query:"-" json:"-"`
}

type NextReadyItem struct {
	ID       string       `json:"id"`
	Code     string       `json:"code"`
	Title    string       `json:"title"`
	Priority TaskPriority `json:"priority"`
	Effort   TaskEffort   `json:"effort"`
	Tags     []string     `json:"tags"`
	Rank     int          `json:"rank"`
	Backlog  string       `json:"backlog"`
	Step     string       `json:"step"`
}

type NextReady struct {
	TotalReady int             `json:"total_ready"`
	Items      []NextReadyItem `json:"items"`

	Fields []string `json:"-"`
}

type nextReadyJSON NextReady

func (n NextReady) MarshalJSON() ([]byte, error) {
	return models.MarshalWithItems(nextReadyJSON(n), n.Fields)
}

type nextReadyRow struct {
	ID       string
	Code     string
	Title    string
	Status   string
	Priority string
	Backlog  string
	Step     string
	Rank     int
	Effort   string
}

func (r nextReadyRow) place() pendingRow {
	return pendingRow{ID: r.ID, Code: r.Code, Title: r.Title, Status: r.Status, Priority: r.Priority, Backlog: r.Backlog, Step: r.Step, Rank: r.Rank}
}

func (m *taskManager) NextReady(ctx context.Context, filter NextReadyFilter) (NextReady, error) {
	fields, err := chooseFields(filter.Fields, NextReadyItem{})
	if err != nil {
		return NextReady{Items: []NextReadyItem{}}, fmt.Errorf("NextReady: %w", err)
	}
	out, err := m.nextReady(ctx, filter)
	out.Fields = fields
	return out, err
}

func (m *taskManager) nextReady(ctx context.Context, filter NextReadyFilter) (NextReady, error) {
	out := NextReady{Items: []NextReadyItem{}}
	if filter.Limit > nextReadyMaxLimit {
		return out, fmt.Errorf("NextReady: %w: limit %d is above %d", ErrInvalidFilter, filter.Limit, nextReadyMaxLimit)
	}
	limit := filter.Limit
	if limit == 0 {
		limit = nextReadyDefaultLimit
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("NextReady: %w", err)
	}
	if err := m.requireSeenBacklog(ctx, backlogs, filter); err != nil {
		return out, fmt.Errorf("NextReady: %w", err)
	}
	query, err := m.scopedTasks(ctx, strings.TrimSpace(filter.Backlog), "", append([]string{tagReady}, filter.Tag...), filter.RequesterID)
	if err != nil {
		return out, fmt.Errorf("NextReady: %w", err)
	}
	if query, err = m.narrowNextReady(ctx, query, filter); err != nil {
		return out, fmt.Errorf("NextReady: %w", err)
	}
	var rows []nextReadyRow
	if err := query.Where("status NOT IN ?", closedStatuses).Select(pendingColumns + ", effort").Order(taskOrder).Scan(&rows).Error; err != nil {
		return out, fmt.Errorf("NextReady: %w", err)
	}
	tags := m.tagNamesOf(ctx, nextReadyIDs(rows))
	for _, row := range rows {
		backlogName, stepName := placeNames(backlogs, row.place())
		if !isToDoStep(backlogs, row.place()) || isMarkedNotReady(tags[row.ID]) {
			continue
		}
		out.TotalReady++
		if len(out.Items) < limit {
			out.Items = append(out.Items, NextReadyItem{
				ID: row.ID, Code: row.Code, Title: row.Title, Priority: TaskPriority(row.Priority), Effort: TaskEffort(row.Effort),
				Tags: tags[row.ID], Rank: row.Rank, Backlog: backlogName, Step: stepName,
			})
		}
	}
	return out, nil
}

func (m *taskManager) requireSeenBacklog(ctx context.Context, backlogs []models.Backlog, filter NextReadyFilter) error {
	name := strings.TrimSpace(filter.Backlog)
	if name == "" {
		return nil
	}
	found, ok := backlogNamed(backlogs, name)
	if ok {
		visible, err := m.backlogVisibility(ctx, filter.RequesterID)
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

func isToDoStep(backlogs []models.Backlog, row pendingRow) bool {
	backlogID, stepID := models.PlaceOf(backlogs, models.Task{Backlog: row.Backlog, Step: row.Step, Status: TaskStatus(row.Status)})
	bi, ok := models.FindBacklog(backlogs, backlogID)
	if !ok {
		return false
	}
	si, ok := models.FindStep(backlogs[bi], stepID)
	if !ok {
		return false
	}
	step := backlogs[bi].Steps[si]
	return strings.EqualFold(step.ID, toDoStepID) || strings.EqualFold(step.Name, toDoStepName)
}

func isMarkedNotReady(tags []string) bool {
	return hasTagFold(tags, tagSinbin) || hasTagFold(tags, tagBlocked) || hasTagFold(tags, tagWaiting)
}

func nextReadyIDs(rows []nextReadyRow) []string {
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

func (f NextReadyFilter) Orchestration() models.OrchestrationFilter {
	return models.OrchestrationFilter{
		Assignee: f.Assignee, Unassigned: f.Unassigned, HasTodos: f.HasTodos, TodosOpen: f.TodosOpen, Effort: f.Effort,
		CreatedSince: f.CreatedSince, UpdatedSince: f.UpdatedSince, HasFiles: f.HasFiles,
	}
}

func (m *taskManager) narrowNextReady(ctx context.Context, q *gorm.DB, filter NextReadyFilter) (*gorm.DB, error) {
	q, err := m.narrowOrchestration(ctx, q, filter.Orchestration())
	if err != nil {
		return nil, err
	}
	if filter.Priority != "" {
		q = q.Where("priority = ?", string(filter.Priority))
	}
	if filter.Repo != "" {
		q = q.Where("project_name = ?", filter.Repo)
	}
	return q, nil
}
