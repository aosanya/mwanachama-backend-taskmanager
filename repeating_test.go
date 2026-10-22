package mwanachamataskmanager

import (
	"errors"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/recur"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func monthlyReview(t *testing.T) RepeatingTask {
	t.Helper()
	rule, err := recur.ParseRule("FREQ=MONTHLY")
	if err != nil {
		t.Fatal(err)
	}
	return RepeatingTask{
		ID:           "rt-1",
		Title:        "Margin review",
		Schedule:     recur.Schedule{Start: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), Zone: time.UTC, Rule: &rule},
		AssigneeKind: models.AssigneeRole,
		AssigneeID:   "role-finance",
	}
}

func TestARepeatingTaskIsDueByTheEndOfItsPeriod(t *testing.T) {
	r := monthlyReview(t)
	o, ok, err := r.OccurrenceAt(time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))
	if err != nil || !ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
	if !o.Start.Equal(time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)) || !o.Due.Equal(time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("period %v..%v", o.Start, o.Due)
	}
	task := o.Task(r)
	if task.Title != "Margin review" || task.DueAt != "2026-04-01T09:00:00Z" {
		t.Fatalf("task %+v", task)
	}
}

func TestOnePeriodHasOneKeyWhateverTheMoment(t *testing.T) {
	r := monthlyReview(t)
	a, _, _ := r.OccurrenceAt(time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC))
	b, _, _ := r.OccurrenceAt(time.Date(2026, 3, 31, 23, 0, 0, 0, time.UTC))
	c, _, _ := r.OccurrenceAt(time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC))
	if a.PeriodKey != b.PeriodKey {
		t.Fatalf("same period, two keys: %s %s", a.PeriodKey, b.PeriodKey)
	}
	if a.PeriodKey == c.PeriodKey {
		t.Fatalf("next period reused key %s", c.PeriodKey)
	}
}

func TestAStoppedRepeatingTaskMakesNoMoreOccurrences(t *testing.T) {
	r := monthlyReview(t).Stop()
	_, ok, err := r.OccurrenceAt(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
	if err != nil || ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
}

func TestNothingIsDueBeforeTheScheduleStarts(t *testing.T) {
	_, ok, err := monthlyReview(t).OccurrenceAt(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || ok {
		t.Fatalf("got ok=%v err=%v", ok, err)
	}
}

func TestARepeatingTaskNeedsAPersonOrRoleAndASchedule(t *testing.T) {
	cases := map[string]func(*RepeatingTask){
		"agent assignee": func(r *RepeatingTask) { r.AssigneeKind = models.AssigneeAgent },
		"no assignee":    func(r *RepeatingTask) { r.AssigneeID = "" },
		"no title":       func(r *RepeatingTask) { r.Title = "" },
		"no schedule":    func(r *RepeatingTask) { r.Schedule = recur.Schedule{} },
	}
	for name, mutate := range cases {
		r := monthlyReview(t)
		mutate(&r)
		if err := r.Validate(); !errors.Is(err, ErrInvalidRepeatingTask) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}
