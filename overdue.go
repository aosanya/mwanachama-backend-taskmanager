package mwanachamataskmanager

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const overdueGraceSetting = "overdue_grace_hours"

var closedStatuses = []string{string(TaskStatusCompleted), string(TaskStatusFailed), string(TaskStatusCancelled)}

func moduleOverdueGrace(s *spec.Spec) (time.Duration, error) {
	if _, declared := s.Setting(overdueGraceSetting); !declared {
		return 0, nil
	}
	effective, err := spec.Resolve(s.Settings, nil, nil, overdueGraceSetting)
	if err != nil {
		return 0, err
	}
	hours, ok := effective.Value.(float64)
	if !ok {
		if n, isInt := effective.Value.(int); isInt {
			hours = float64(n)
		} else {
			return 0, fmt.Errorf("%s: %v is not a number of hours", overdueGraceSetting, effective.Value)
		}
	}
	if hours < 0 {
		return 0, fmt.Errorf("%s: %v is negative", overdueGraceSetting, hours)
	}
	return time.Duration(hours) * time.Hour, nil
}

func (m *taskManager) ListOverdueTasks(ctx context.Context, filter OverdueFilter) ([]OverdueTask, error) {
	asOf := time.Now().UTC()
	if filter.AsOf != "" {
		parsed, err := time.Parse(time.RFC3339, filter.AsOf)
		if err != nil {
			return nil, fmt.Errorf("ListOverdueTasks: as_of %q: %w", filter.AsOf, ErrInvalidOverdueFilter)
		}
		asOf = parsed.UTC()
	}
	q := m.store.Query(ctx, roleTask).
		Where("deleted = ?", false).
		Where("due_at <> ?", "").
		Where("status NOT IN ?", closedStatuses)
	tasks, err := specstore.List[Task](m.store, q, roleTask)
	if err != nil {
		return nil, fmt.Errorf("ListOverdueTasks: %w", err)
	}
	states, err := m.taskStates(ctx)
	if err != nil {
		return nil, fmt.Errorf("ListOverdueTasks: %w", err)
	}
	cutoff := asOf.Add(-m.overdueGrace)
	var overdue []OverdueTask
	dueAt := map[string]time.Time{}
	for _, t := range tasks {
		due, err := time.Parse(time.RFC3339, t.DueAt)
		if err != nil || !due.Before(cutoff) {
			continue
		}
		dueAt[t.ID] = due
		t.State = models.StateNameFor(states, t.Status)
		overdue = append(overdue, OverdueTask{Task: t, OverdueHours: int(math.Floor(asOf.Sub(due).Hours()))})
	}
	sort.SliceStable(overdue, func(i, j int) bool {
		return dueAt[overdue[i].ID].Before(dueAt[overdue[j].ID])
	})
	limit := filter.Limit
	if limit <= 0 || limit > maxListPage {
		limit = maxListPage
	}
	if len(overdue) > limit {
		overdue = overdue[:limit]
	}
	for i := range overdue {
		overdue[i].Tags = m.loadTagNames(ctx, overdue[i].ID)
	}
	if overdue == nil {
		overdue = []OverdueTask{}
	}
	return overdue, nil
}
