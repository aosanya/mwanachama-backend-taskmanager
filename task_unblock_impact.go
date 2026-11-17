package mwanachamataskmanager

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	unblockImpactDefaultLimit = 10
	unblockImpactMaxLimit     = 50
	unblockImpactChunk        = 500
)

type UnblockImpactFilter struct {
	Limit   int            `query:"limit,min=1"`
	Tag     models.TagList `query:"tag"`
	Backlog string         `query:"backlog"`

	RequesterID string `query:"-" json:"-"`
}

type UnblockImpactItem struct {
	ID         string       `json:"id"`
	Code       string       `json:"code"`
	Title      string       `json:"title"`
	Step       string       `json:"step"`
	Priority   TaskPriority `json:"priority"`
	Direct     int          `json:"direct"`
	Total      int          `json:"total"`
	Dependents []string     `json:"dependents"`
}

type UnblockImpact struct {
	Items []UnblockImpactItem `json:"items"`
}

type dependencyEdge struct {
	FromID string
	ToID   string
}

func (m *taskManager) UnblockImpact(ctx context.Context, filter UnblockImpactFilter) (UnblockImpact, error) {
	out := UnblockImpact{Items: []UnblockImpactItem{}}
	limit := filter.Limit
	if limit > unblockImpactMaxLimit {
		return out, fmt.Errorf("UnblockImpact: %w: limit %d is above %d", ErrInvalidFilter, limit, unblockImpactMaxLimit)
	}
	if limit == 0 {
		limit = unblockImpactDefaultLimit
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("UnblockImpact: %w", err)
	}
	if err := m.requireSeenBacklog(ctx, backlogs, NextReadyFilter{Backlog: filter.Backlog, RequesterID: filter.RequesterID}); err != nil {
		return out, fmt.Errorf("UnblockImpact: %w", err)
	}
	edges, err := m.dependencyEdges(ctx)
	if err != nil {
		return out, fmt.Errorf("UnblockImpact: %w", err)
	}
	ends := endsOf(edges)
	open, err := m.openAmong(ctx, backlogs, ends, "", nil, filter.RequesterID)
	if err != nil {
		return out, fmt.Errorf("UnblockImpact: %w", err)
	}
	listed := open
	if strings.TrimSpace(filter.Backlog) != "" || len(filter.Tag) > 0 {
		listed, err = m.openAmong(ctx, backlogs, ends, strings.TrimSpace(filter.Backlog), filter.Tag, filter.RequesterID)
		if err != nil {
			return out, fmt.Errorf("UnblockImpact: %w", err)
		}
	}
	waiters := waitersOf(edges, open)
	for id, task := range listed {
		direct := waiters[id]
		if len(direct) == 0 {
			continue
		}
		downstream := reachableFrom(id, waiters)
		codes := make([]string, 0, len(downstream))
		for _, waiter := range downstream {
			codes = append(codes, open[waiter].Code)
		}
		slices.Sort(codes)
		_, step := placeNames(backlogs, pendingRow{Status: string(task.Status), Backlog: task.Backlog, Step: task.Step})
		out.Items = append(out.Items, UnblockImpactItem{
			ID: task.ID, Code: task.Code, Title: task.Title, Step: step, Priority: task.Priority,
			Direct: len(direct), Total: len(downstream), Dependents: codes,
		})
	}
	rankOf := func(item UnblockImpactItem) int { return open[item.ID].Rank }
	slices.SortFunc(out.Items, func(a, b UnblockImpactItem) int {
		switch {
		case a.Total != b.Total:
			return b.Total - a.Total
		case a.Direct != b.Direct:
			return b.Direct - a.Direct
		case rankOf(a) != rankOf(b):
			return rankOf(a) - rankOf(b)
		}
		return strings.Compare(a.Code, b.Code)
	})
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
	}
	return out, nil
}

func (m *taskManager) dependencyEdges(ctx context.Context) ([]dependencyEdge, error) {
	spec := m.relSpecs()[RelLabelDependsOn]
	edges := []dependencyEdge{}
	err := m.onTable(m.db.WithContext(ctx), spec.joinTable).
		Select(spec.joinFromCol + " AS from_id, " + spec.joinToCol + " AS to_id").
		Scan(&edges).Error
	return edges, err
}

func (m *taskManager) openAmong(ctx context.Context, backlogs []models.Backlog, ids []string, backlog string, tags []string, requesterID string) (map[string]Task, error) {
	found := map[string]Task{}
	for start := 0; start < len(ids); start += unblockImpactChunk {
		chunk := ids[start:min(start+unblockImpactChunk, len(ids))]
		query, err := m.scopedTasks(ctx, backlog, "", tags, requesterID)
		if err != nil {
			return nil, err
		}
		tasks, err := specstore.List[Task](m.store, query.Where("id IN ? AND status NOT IN ?", chunk, closedStatuses), roleTask)
		if err != nil {
			return nil, err
		}
		for _, task := range tasks {
			if !atClosedStep(backlogs, task) {
				found[task.ID] = task
			}
		}
	}
	return found, nil
}

var closedStepNames = []string{"done", "archive", "cancelled", "canceled"}

func atClosedStep(backlogs []models.Backlog, task Task) bool {
	backlogID, stepID := models.PlaceOf(backlogs, models.Task{Backlog: task.Backlog, Step: task.Step, Status: task.Status})
	index, ok := models.FindBacklog(backlogs, backlogID)
	if !ok {
		return false
	}
	position, ok := models.FindStep(backlogs[index], stepID)
	if !ok {
		return false
	}
	step := backlogs[index].Steps[position]
	return slices.Contains(closedStepNames, strings.ToLower(step.ID)) || slices.Contains(closedStepNames, strings.ToLower(step.Name))
}

func endsOf(edges []dependencyEdge) []string {
	seen := map[string]bool{}
	ids := []string{}
	for _, edge := range edges {
		for _, id := range []string{edge.FromID, edge.ToID} {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func waitersOf(edges []dependencyEdge, open map[string]Task) map[string][]string {
	waiters := map[string][]string{}
	seen := map[dependencyEdge]bool{}
	for _, edge := range edges {
		_, waiterOpen := open[edge.FromID]
		_, blockerOpen := open[edge.ToID]
		if !waiterOpen || !blockerOpen || edge.FromID == edge.ToID || seen[edge] {
			continue
		}
		seen[edge] = true
		waiters[edge.ToID] = append(waiters[edge.ToID], edge.FromID)
	}
	return waiters
}

func reachableFrom(start string, waiters map[string][]string) []string {
	visited := map[string]bool{start: true}
	reached := []string{}
	queue := []string{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, waiter := range waiters[current] {
			if visited[waiter] {
				continue
			}
			visited[waiter] = true
			reached = append(reached, waiter)
			queue = append(queue, waiter)
		}
	}
	return reached
}
