package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/recur"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

type (
	RepeatingTaskRecord = models.RepeatingTaskRecord
	RepeatingTaskPeriod = models.RepeatingTaskPeriod
	RepeatingTaskFilter = models.RepeatingTaskFilter
)

var ErrRepeatingTaskNotFound = errors.New("repeating task not found")

func RecordOf(r RepeatingTask) RepeatingTaskRecord {
	rec := RepeatingTaskRecord{
		ID:            r.ID,
		Title:         r.Title,
		Description:   r.Description,
		ScheduleStart: formatInstant(r.Schedule.Start),
		ScheduleEnd:   formatInstant(r.Schedule.End),
		AssigneeKind:  r.AssigneeKind,
		AssigneeID:    r.AssigneeID,
	}
	if r.Schedule.Zone != nil {
		rec.ScheduleZone = r.Schedule.Zone.String()
	}
	if r.Schedule.Rule != nil {
		rec.ScheduleRule = r.Schedule.Rule.String()
	}
	for _, d := range r.Schedule.Dates {
		rec.ScheduleDates = append(rec.ScheduleDates, d.String())
	}
	return rec
}

func RepeatingTaskFromRecord(rec RepeatingTaskRecord) (RepeatingTask, error) {
	r := RepeatingTask{
		ID:           rec.ID,
		Title:        rec.Title,
		Description:  rec.Description,
		AssigneeKind: rec.AssigneeKind,
		AssigneeID:   rec.AssigneeID,
		Stopped:      rec.StoppedAt != "",
	}
	zone, err := time.LoadLocation(rec.ScheduleZone)
	if err != nil {
		return RepeatingTask{}, fmt.Errorf("%w: zone %q: %v", ErrInvalidRepeatingTask, rec.ScheduleZone, err)
	}
	r.Schedule.Zone = zone
	if r.Schedule.Start, err = parseInstant(rec.ScheduleStart); err != nil {
		return RepeatingTask{}, err
	}
	if r.Schedule.End, err = parseInstant(rec.ScheduleEnd); err != nil {
		return RepeatingTask{}, err
	}
	if rec.ScheduleRule != "" {
		rule, err := recur.ParseRule(rec.ScheduleRule)
		if err != nil {
			return RepeatingTask{}, fmt.Errorf("%w: %v", ErrInvalidRepeatingTask, err)
		}
		r.Schedule.Rule = &rule
	}
	for _, s := range rec.ScheduleDates {
		d, err := recur.ParseDate(s)
		if err != nil {
			return RepeatingTask{}, fmt.Errorf("%w: %v", ErrInvalidRepeatingTask, err)
		}
		r.Schedule.Dates = append(r.Schedule.Dates, d)
	}
	return r, nil
}

func formatInstant(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func parseInstant(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %q is not an instant", ErrInvalidRepeatingTask, s)
	}
	return t, nil
}

func (m *taskManager) CreateRepeatingTask(ctx context.Context, r RepeatingTask) (RepeatingTaskRecord, error) {
	r.ID = newSpecID()
	r.Stopped = false
	if err := r.Validate(); err != nil {
		return RepeatingTaskRecord{}, err
	}
	rec := RecordOf(r)
	if _, err := RepeatingTaskFromRecord(rec); err != nil {
		return RepeatingTaskRecord{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rec.CreatedAt = now
	rec.UpdatedAt = now
	if err := m.store.Insert(ctx, roleRepeatingTask, rec); err != nil {
		return RepeatingTaskRecord{}, fmt.Errorf("CreateRepeatingTask: %w", err)
	}
	return rec, nil
}

func (m *taskManager) GetRepeatingTask(ctx context.Context, id string) (RepeatingTaskRecord, error) {
	var rec RepeatingTaskRecord
	q := m.store.Query(ctx, roleRepeatingTask).Where("id = ?", id)
	if err := m.store.Take(q, roleRepeatingTask, &rec, ErrRepeatingTaskNotFound); err != nil {
		if errors.Is(err, ErrRepeatingTaskNotFound) {
			return RepeatingTaskRecord{}, err
		}
		return RepeatingTaskRecord{}, fmt.Errorf("GetRepeatingTask: %w", err)
	}
	return rec, nil
}

func (m *taskManager) ListRepeatingTasks(ctx context.Context, filter RepeatingTaskFilter) ([]RepeatingTaskRecord, error) {
	q := m.store.Query(ctx, roleRepeatingTask)
	if filter.AssigneeKind != "" {
		q = q.Where("assignee_kind = ?", string(filter.AssigneeKind))
	}
	if filter.AssigneeID != "" {
		q = q.Where("assignee_id = ?", filter.AssigneeID)
	}
	q = q.Order("created_at").Order("id").Limit(maxListPage)
	out, err := specstore.List[RepeatingTaskRecord](m.store, q, roleRepeatingTask)
	if err != nil {
		return nil, fmt.Errorf("ListRepeatingTasks: %w", err)
	}
	return out, nil
}

func (m *taskManager) StopRepeatingTask(ctx context.Context, id string) (RepeatingTaskRecord, error) {
	rec, err := m.GetRepeatingTask(ctx, id)
	if err != nil {
		return RepeatingTaskRecord{}, err
	}
	if rec.StoppedAt != "" {
		return rec, nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res := m.store.Query(ctx, roleRepeatingTask).
		Where("id = ?", id).
		Updates(map[string]any{"stopped_at": now, "updated_at": now})
	if res.Error != nil {
		return RepeatingTaskRecord{}, fmt.Errorf("StopRepeatingTask: %w", res.Error)
	}
	return m.GetRepeatingTask(ctx, id)
}

func (m *taskManager) MakeRepeatingTaskDue(ctx context.Context, id string, at time.Time) (RepeatingTaskPeriod, bool, error) {
	rec, err := m.GetRepeatingTask(ctx, id)
	if err != nil {
		return RepeatingTaskPeriod{}, false, err
	}
	r, err := RepeatingTaskFromRecord(rec)
	if err != nil {
		return RepeatingTaskPeriod{}, false, err
	}
	occ, ok, err := r.OccurrenceAt(at)
	if err != nil || !ok {
		return RepeatingTaskPeriod{}, false, err
	}
	var made RepeatingTaskPeriod
	q := m.store.Query(ctx, roleRepeatingPeriod).Where("period_key = ?", occ.PeriodKey)
	switch err := m.store.Take(q, roleRepeatingPeriod, &made, ErrRepeatingTaskNotFound); {
	case err == nil:
		return made, false, nil
	case !errors.Is(err, ErrRepeatingTaskNotFound):
		return RepeatingTaskPeriod{}, false, fmt.Errorf("MakeRepeatingTaskDue: %w", err)
	}
	task, err := m.CreateTask(ctx, occ.Task(r))
	if err != nil {
		return RepeatingTaskPeriod{}, false, err
	}
	if err := m.AssignTaskToHolder(ctx, task.ID, r.AssigneeKind, r.AssigneeID); err != nil {
		return RepeatingTaskPeriod{}, false, err
	}
	made = RepeatingTaskPeriod{
		PeriodKey:       occ.PeriodKey,
		RepeatingTaskID: r.ID,
		TaskID:          task.ID,
		PeriodStart:     formatInstant(occ.Start),
		DueAt:           formatInstant(occ.Due),
		MadeAt:          time.Now().UTC().Format(time.RFC3339),
	}
	if err := m.store.Insert(ctx, roleRepeatingPeriod, made); err != nil {
		return RepeatingTaskPeriod{}, false, fmt.Errorf("MakeRepeatingTaskDue: %w", err)
	}
	return made, true, nil
}
