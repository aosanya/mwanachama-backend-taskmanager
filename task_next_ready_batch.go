package mwanachamataskmanager

import (
	"context"
	"fmt"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"strings"
)

const (
	nextReadyBatchDefaultCount        = 3
	nextReadyBatchMaxCount            = 10
	nextReadyBatchDefaultSkippedLimit = 3
	nextReadyBatchMaxSkippedLimit     = 50
)

type NextReadyBatchFilter struct {
	Count          int            `query:"count,min=1"`
	SkippedLimit   int            `query:"skipped_limit,min=1"`
	Tag            models.TagList `query:"tag"`
	Backlog        string         `query:"backlog"`
	IncludeUnfiled bool           `query:"include_unfiled"`
	RequesterID    string         `query:"-" json:"-"`
}
type NextReadyBatchItem struct {
	ID       string       `json:"id"`
	Code     string       `json:"code"`
	Title    string       `json:"title"`
	Priority TaskPriority `json:"priority"`
	Effort   TaskEffort   `json:"effort"`
	Files    []string     `json:"files"`
}
type NextReadyBatchSkip struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	ClashesWith string `json:"clashes_with"`
}
type NextReadyBatch struct {
	Items          []NextReadyBatchItem `json:"items"`
	Skipped        []NextReadyBatchSkip `json:"skipped"`
	SkippedTotal   int                  `json:"skipped_total"`
	UnfiledLeftOut int                  `json:"unfiled_left_out"`
}

func (m *taskManager) NextReadyBatch(ctx context.Context, filter NextReadyBatchFilter) (NextReadyBatch, error) {
	out := NextReadyBatch{Items: []NextReadyBatchItem{}, Skipped: []NextReadyBatchSkip{}}
	count := filter.Count
	if count > nextReadyBatchMaxCount {
		return out, fmt.Errorf("NextReadyBatch: %w: count %d is above %d", ErrInvalidFilter, count, nextReadyBatchMaxCount)
	}
	if count == 0 {
		count = nextReadyBatchDefaultCount
	}
	skippedLimit, err := resolveSkippedLimit(filter.SkippedLimit)
	if err != nil {
		return out, fmt.Errorf("NextReadyBatch: %w", err)
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return out, fmt.Errorf("NextReadyBatch: %w", err)
	}
	if err := m.requireSeenBacklog(ctx, backlogs, NextReadyFilter{Backlog: filter.Backlog, RequesterID: filter.RequesterID}); err != nil {
		return out, fmt.Errorf("NextReadyBatch: %w", err)
	}
	candidates, err := m.readyCandidates(ctx, backlogs, filter)
	if err != nil {
		return out, fmt.Errorf("NextReadyBatch: %w", err)
	}
	running, err := m.inProgressWithFiles(ctx, FileOverlapsFilter{RequesterID: filter.RequesterID})
	if err != nil {
		return out, fmt.Errorf("NextReadyBatch: %w", err)
	}
	chosen := []Task{}
	for _, task := range candidates {
		if len(task.Files) == 0 && !filter.IncludeUnfiled {
			out.UnfiledLeftOut++
			continue
		}
		if len(out.Items) >= count {
			continue
		}
		if clash, found := firstClash(task, running, chosen); found {
			out.SkippedTotal++
			if len(out.Skipped) < skippedLimit {
				out.Skipped = append(out.Skipped, NextReadyBatchSkip{ID: task.ID, Code: task.Code, Title: task.Title, ClashesWith: clash})
			}
			continue
		}
		chosen = append(chosen, task)
		out.Items = append(out.Items, NextReadyBatchItem{ID: task.ID, Code: task.Code, Title: task.Title, Priority: task.Priority, Effort: task.Effort, Files: append([]string{}, task.Files...)})
	}
	return out, nil
}
func resolveSkippedLimit(requested int) (int, error) {
	if requested < 0 || requested > nextReadyBatchMaxSkippedLimit {
		return 0, fmt.Errorf("%w: skipped_limit %d is not between 1 and %d", ErrInvalidFilter, requested, nextReadyBatchMaxSkippedLimit)
	}
	if requested == 0 {
		return nextReadyBatchDefaultSkippedLimit, nil
	}
	return requested, nil
}
func (m *taskManager) readyCandidates(ctx context.Context, backlogs []models.Backlog, filter NextReadyBatchFilter) ([]Task, error) {
	query, err := m.scopedTasks(ctx, strings.TrimSpace(filter.Backlog), "", append([]string{tagReady}, filter.Tag...), filter.RequesterID)
	if err != nil {
		return nil, err
	}
	tasks, err := specstore.List[Task](m.store, query.Where("status NOT IN ?", closedStatuses).Order(taskOrder).Limit(maxListPage), roleTask)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	tags := m.tagNamesOf(ctx, ids)
	ready := []Task{}
	for _, task := range tasks {
		place := pendingRow{Status: string(task.Status), Backlog: task.Backlog, Step: task.Step}
		if isToDoStep(backlogs, place) && !isMarkedNotReady(tags[task.ID]) {
			ready = append(ready, task)
		}
	}
	return ready, nil
}
func firstClash(task Task, running, chosen []Task) (string, bool) {
	for _, other := range append(append([]Task{}, running...), chosen...) {
		if other.ID != task.ID && len(overlappingPairs(task.Files, other.Files)) > 0 {
			return taskDisplayName(other), true
		}
	}
	return "", false
}
