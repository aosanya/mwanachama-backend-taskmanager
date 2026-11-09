package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const defaultBacklogName = "default"

var (
	ErrUnknownBacklog = errors.New("unknown backlog")
	ErrInvalidFilter  = errors.New("invalid list filter")
)

func (m *taskManager) narrowList(ctx context.Context, q *gorm.DB, filter TaskFilter, states []models.TaskState, now time.Time) (*gorm.DB, error) {
	if filter.UntouchedDays < 0 {
		return nil, fmt.Errorf("%w: untouched_days %d is negative", ErrInvalidFilter, filter.UntouchedDays)
	}
	if filter.Repo != "" {
		q = q.Where("project_name = ?", filter.Repo)
	}
	if filter.Backlog != "" && !strings.EqualFold(filter.Backlog, defaultBacklogName) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownBacklog, filter.Backlog)
	}
	if filter.Step != "" {
		statuses, ok := statusesOfStep(states, filter.Step)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownTaskState, filter.Step)
		}
		q = q.Where("status IN ?", statuses)
	}
	for _, tag := range filter.Tag {
		q = q.Where("id IN (?)", m.taskIDsWithTag(ctx, tag))
	}
	if filter.UntouchedDays > 0 {
		cutoff := now.AddDate(0, 0, -filter.UntouchedDays).Format(time.RFC3339)
		q = q.Where("updated_at < ?", cutoff)
	}
	return q, nil
}

func statusesOfStep(states []models.TaskState, step string) ([]string, bool) {
	for _, s := range states {
		if !strings.EqualFold(s.Name, step) {
			continue
		}
		out := make([]string, 0, len(s.Statuses))
		for _, status := range s.Statuses {
			out = append(out, string(status))
		}
		return out, true
	}
	return nil, false
}

func (m *taskManager) taskIDsWithTag(ctx context.Context, name string) *gorm.DB {
	tagIDs := m.store.Query(ctx, roleTag).Select("id").Where("lower(name) = ?", strings.ToLower(name))
	return m.store.Query(ctx, roleTagging).Select("task_id").Where("tag_id IN (?)", tagIDs)
}
