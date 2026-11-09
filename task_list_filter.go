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

var (
	ErrUnknownBacklog = errors.New("unknown backlog")
	ErrInvalidFilter  = errors.New("invalid list filter")
)

func (m *taskManager) narrowList(ctx context.Context, q *gorm.DB, filter TaskFilter, now time.Time) (*gorm.DB, error) {
	if filter.UntouchedDays < 0 {
		return nil, fmt.Errorf("%w: untouched_days %d is negative", ErrInvalidFilter, filter.UntouchedDays)
	}
	if filter.Repo != "" {
		q = q.Where("project_name = ?", filter.Repo)
	}
	q, err := m.narrowToPlace(ctx, q, filter)
	if err != nil {
		return nil, err
	}
	for _, tag := range filter.Tag {
		if tag == "" {
			continue
		}
		q = q.Where("id IN (?)", m.taskIDsWithTag(ctx, tag))
	}
	if filter.UntouchedDays > 0 {
		cutoff := now.AddDate(0, 0, -filter.UntouchedDays).Format(time.RFC3339)
		q = q.Where("updated_at < ?", cutoff)
	}
	return q, nil
}

func (m *taskManager) narrowToPlace(ctx context.Context, q *gorm.DB, filter TaskFilter) (*gorm.DB, error) {
	if filter.Backlog == "" && filter.Step == "" {
		return q, nil
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return nil, err
	}
	backlog := backlogs[0]
	if filter.Backlog != "" {
		found, ok := backlogNamed(backlogs, filter.Backlog)
		if !ok {
			return nil, fmt.Errorf("%w: %q", ErrUnknownBacklog, filter.Backlog)
		}
		backlog = found
	}
	isFirst := backlog.ID == backlogs[0].ID
	if filter.Step == "" {
		return q.Where(inBacklog(isFirst), backlog.ID), nil
	}
	step, ok := stepNamed(backlog, filter.Step)
	if !ok {
		return nil, fmt.Errorf("%w: %q in %q", ErrUnknownBacklogStep, filter.Step, backlog.Name)
	}
	if !isFirst || len(step.Statuses) == 0 {
		return q.Where("backlog = ? AND step = ?", backlog.ID, step.ID), nil
	}
	statuses := make([]string, len(step.Statuses))
	for i, status := range step.Statuses {
		statuses[i] = string(status)
	}
	return q.Where("(backlog = ? AND step = ?) OR ((backlog = '' OR backlog IS NULL) AND status IN ?)", backlog.ID, step.ID, statuses), nil
}

func inBacklog(isFirst bool) string {
	if isFirst {
		return "(backlog = ? OR backlog = '' OR backlog IS NULL)"
	}
	return "backlog = ?"
}

func backlogNamed(backlogs []models.Backlog, name string) (models.Backlog, bool) {
	for _, b := range backlogs {
		if strings.EqualFold(b.ID, name) || strings.EqualFold(b.Name, name) {
			return b, true
		}
	}
	return models.Backlog{}, false
}

func stepNamed(b models.Backlog, name string) (models.BacklogStep, bool) {
	for _, s := range b.Steps {
		if strings.EqualFold(s.ID, name) || strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return models.BacklogStep{}, false
}

func (m *taskManager) taskIDsWithTag(ctx context.Context, name string) *gorm.DB {
	tagIDs := m.store.Query(ctx, roleTag).Select("id").Where("lower(name) = ?", strings.ToLower(name))
	return m.store.Query(ctx, roleTagging).Select("task_id").Where("tag_id IN (?)", tagIDs)
}
