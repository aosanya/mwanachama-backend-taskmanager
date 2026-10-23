package mwanachamataskmanager

import (
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/recur"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

var ErrInvalidRepeatingTask = errors.New("a repeating task needs an id, a title, a valid schedule and an actor or role assignee")

type RepeatingTask struct {
	ID           string
	Title        string
	Description  string
	Schedule     recur.Schedule
	AssigneeKind models.AssigneeKind
	AssigneeID   string
	Stopped      bool
}

type Occurrence struct {
	RepeatingTaskID string
	PeriodKey       string
	Start           time.Time
	Due             time.Time
}

func (r RepeatingTask) Validate() error {
	if r.ID == "" || r.Title == "" {
		return ErrInvalidRepeatingTask
	}
	if r.AssigneeKind != models.AssigneeActor && r.AssigneeKind != models.AssigneeRole {
		return ErrInvalidRepeatingTask
	}
	if r.AssigneeID == "" {
		return ErrInvalidRepeatingTask
	}
	if err := r.Schedule.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRepeatingTask, err)
	}
	if len(r.Schedule.Dates) > 0 && r.Schedule.End.IsZero() {
		return fmt.Errorf("%w: a list of dates needs an end, which is when the last date's task is due", ErrInvalidRepeatingTask)
	}
	return nil
}

func (r RepeatingTask) OccurrenceAt(at time.Time) (Occurrence, bool, error) {
	if err := r.Validate(); err != nil {
		return Occurrence{}, false, err
	}
	if r.Stopped {
		return Occurrence{}, false, nil
	}
	start, end, ok := r.Schedule.Period(at)
	if !ok || end.IsZero() {
		return Occurrence{}, false, nil
	}
	return Occurrence{
		RepeatingTaskID: r.ID,
		PeriodKey:       r.ID + "@" + start.UTC().Format(time.RFC3339),
		Start:           start,
		Due:             end,
	}, true, nil
}

func (o Occurrence) Task(r RepeatingTask) models.Task {
	return models.Task{
		Title:       r.Title,
		Description: r.Description,
		DueAt:       o.Due.UTC().Format(time.RFC3339Nano),
	}
}

func (r RepeatingTask) Stop() RepeatingTask {
	r.Stopped = true
	return r
}
