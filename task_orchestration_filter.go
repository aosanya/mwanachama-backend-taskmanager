package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const maxFilterIDs = 100

const priorityRank = `CASE priority WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 ELSE 4 END`

var sortOrders = map[string]string{
	"":         "",
	"rank":     "",
	"priority": priorityRank,
	"updated":  "updated_at DESC",
	"created":  "created_at DESC",
}

var knownEfforts = map[string]bool{
	string(models.TaskEffortUnknown):  true,
	string(models.TaskEffortVeryLow):  true,
	string(models.TaskEffortLow):      true,
	string(models.TaskEffortMedium):   true,
	string(models.TaskEffortHigh):     true,
	string(models.TaskEffortVeryHigh): true,
}

func (m *taskManager) narrowOrchestration(ctx context.Context, q *gorm.DB, f models.OrchestrationFilter) (*gorm.DB, error) {
	if err := checkOrchestration(f); err != nil {
		return nil, err
	}
	q = narrowByHolder(q, f)
	q = m.narrowByMarks(ctx, q, f)
	q = m.narrowByTodos(ctx, q, f)
	q = narrowByContent(q, f)
	return narrowBySince(q, f), nil
}

func checkOrchestration(f models.OrchestrationFilter) error {
	if f.Assignee != "" && f.Unassigned {
		return fmt.Errorf("%w: assignee and unassigned cannot be asked together", ErrInvalidFilter)
	}
	if f.Effort != "" && !knownEfforts[f.Effort] {
		return fmt.Errorf("%w: effort %q", ErrInvalidFilter, f.Effort)
	}
	if _, ok := sortOrders[f.Sort]; !ok {
		return fmt.Errorf("%w: sort %q", ErrInvalidFilter, f.Sort)
	}
	if len(f.IDs) > maxFilterIDs {
		return fmt.Errorf("%w: %d ids is above %d", ErrInvalidFilter, len(f.IDs), maxFilterIDs)
	}
	for name, value := range map[string]string{"created_since": f.CreatedSince, "updated_since": f.UpdatedSince} {
		if _, err := sinceInstant(value); err != nil {
			return fmt.Errorf("%w: %s %q is not RFC 3339", ErrInvalidFilter, name, value)
		}
	}
	return nil
}

func sinceInstant(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", err
	}
	return at.UTC().Format(time.RFC3339), nil
}

func narrowByHolder(q *gorm.DB, f models.OrchestrationFilter) *gorm.DB {
	if f.Assignee != "" {
		q = q.Where("assigned_to = ?", f.Assignee)
	}
	if f.Unassigned {
		q = q.Where("(assigned_to IS NULL OR assigned_to = '')")
	}
	return q
}

func (m *taskManager) narrowByMarks(ctx context.Context, q *gorm.DB, f models.OrchestrationFilter) *gorm.DB {
	blocked := m.taskIDsWithTag(ctx, tagBlocked)
	sinbin := m.taskIDsWithTag(ctx, tagSinbin)
	waiting := m.taskIDsWithTag(ctx, tagWaiting)
	if f.Blocked != nil {
		q = q.Where(chosen(*f.Blocked, "id IN (?)", "id NOT IN (?)"), blocked)
	}
	if f.Sinbin != nil {
		q = q.Where(chosen(*f.Sinbin, "(id IN (?) AND id NOT IN (?))", "(id NOT IN (?) OR id IN (?))"), sinbin, blocked)
	}
	if f.Waiting != nil {
		q = q.Where(chosen(*f.Waiting, "(id IN (?) AND id NOT IN (?) AND id NOT IN (?))", "(id NOT IN (?) OR id IN (?) OR id IN (?))"), waiting, blocked, sinbin)
	}
	return q
}

func chosen(wanted bool, yes, no string) string {
	if wanted {
		return yes
	}
	return no
}

func (m *taskManager) narrowByTodos(ctx context.Context, q *gorm.DB, f models.OrchestrationFilter) *gorm.DB {
	liveParents := func() *gorm.DB {
		return m.store.Query(ctx, roleTaskTodo).Select("parent_task_id").Where("deleted = ? AND parent_task_id IS NOT NULL", false)
	}
	if f.HasTodos != nil {
		q = q.Where(chosen(*f.HasTodos, "id IN (?)", "id NOT IN (?)"), liveParents())
	}
	if f.TodosOpen {
		q = q.Where("id IN (?)", liveParents().Where("status NOT IN ?", finishedTodoStatuses))
	}
	return q
}

func narrowByContent(q *gorm.DB, f models.OrchestrationFilter) *gorm.DB {
	if f.Effort == string(models.TaskEffortUnknown) {
		q = q.Where("(effort = ? OR effort = '' OR effort IS NULL)", f.Effort)
	} else if f.Effort != "" {
		q = q.Where("effort = ?", f.Effort)
	}
	if f.HasFiles != nil {
		q = q.Where(chosen(*f.HasFiles, "COALESCE(CAST(files AS TEXT), '') NOT IN ('', '[]', 'null')", "COALESCE(CAST(files AS TEXT), '') IN ('', '[]', 'null')"))
	}
	if len(f.IDs) > 0 {
		q = q.Where("id IN ?", f.IDs)
	}
	return q
}

func narrowBySince(q *gorm.DB, f models.OrchestrationFilter) *gorm.DB {
	if created, _ := sinceInstant(f.CreatedSince); created != "" {
		q = q.Where("created_at >= ?", created)
	}
	if updated, _ := sinceInstant(f.UpdatedSince); updated != "" {
		q = q.Where("updated_at >= ?", updated)
	}
	return q
}

func sortedBy(sort, tail string) string {
	if lead := sortOrders[sort]; lead != "" {
		return lead + ", " + tail
	}
	return tail
}
