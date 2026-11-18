package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/recur"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func weeklyCount() mwanachamataskmanager.NewRepeatingTask {
	return mwanachamataskmanager.NewRepeatingTask{
		Title:        "Weekly count",
		Description:  "Count the stalls.",
		Schedule:     recur.Wire{Start: "2026-01-05T08:00:00Z", Zone: "Europe/Budapest", Rule: "FREQ=WEEKLY;BYDAY=MO"},
		AssigneeKind: models.AssigneeRole,
		AssigneeID:   "role-9",
	}
}

func TestAnUpdatedRepeatingTaskKeepsItsIdAndTakesTheNewSchedule(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	created, err := m.StartRepeatingTask(ctx, weeklyCount())
	if err != nil {
		t.Fatal(err)
	}
	change := weeklyCount()
	change.Title = "Fortnightly count"
	change.Description = "Count the stalls and the bins."
	change.Schedule = recur.Wire{Start: "2026-02-04T09:30:00Z", End: "2027-01-01T00:00:00Z", Zone: "Europe/Budapest", Rule: "FREQ=WEEKLY;INTERVAL=2;BYDAY=WE"}
	change.AssigneeKind = models.AssigneeActor
	change.AssigneeID = "actor-4"

	updated, err := m.UpdateRepeatingTask(ctx, created.ID, change)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID {
		t.Fatalf("id changed from %s to %s", created.ID, updated.ID)
	}
	got, err := m.GetRepeatingTask(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Fortnightly count" || got.Description != "Count the stalls and the bins." || got.AssigneeKind != models.AssigneeActor || got.AssigneeID != "actor-4" {
		t.Fatalf("fields not taken: %+v", got)
	}
	if got.ScheduleRule != "FREQ=WEEKLY;INTERVAL=2;BYDAY=WE" || got.ScheduleStart != "2026-02-04T09:30:00Z" || got.ScheduleEnd != "2027-01-01T00:00:00Z" || got.ScheduleZone != "Europe/Budapest" {
		t.Fatalf("schedule not taken: %+v", got)
	}
	if got.CreatedAt != created.CreatedAt {
		t.Fatalf("created_at changed from %s to %s", created.CreatedAt, got.CreatedAt)
	}
	if got.StoppedAt != "" {
		t.Fatalf("an update must not stop the schedule: %+v", got)
	}
}

func TestAnUpdatedRepeatingTaskKeepsTheJobsItAlreadyMade(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	created, err := m.StartRepeatingTask(ctx, weeklyCount())
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)
	first, made, err := m.MakeRepeatingTaskDue(ctx, created.ID, at)
	if err != nil || !made {
		t.Fatalf("first period: %+v made %v err %v", first, made, err)
	}
	change := weeklyCount()
	change.Title = "Weekly count of stalls"
	if _, err := m.UpdateRepeatingTask(ctx, created.ID, change); err != nil {
		t.Fatal(err)
	}
	again, madeAgain, err := m.MakeRepeatingTaskDue(ctx, created.ID, at)
	if err != nil {
		t.Fatal(err)
	}
	if madeAgain || again.TaskID != first.TaskID {
		t.Fatalf("the period already made was made again: first %+v again %+v made %v", first, again, madeAgain)
	}
}

func TestAStoppedRepeatingTaskCannotBeUpdated(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	created, err := m.StartRepeatingTask(ctx, weeklyCount())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.StopRepeatingTask(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	change := weeklyCount()
	change.Title = "Revived"
	_, err = m.UpdateRepeatingTask(ctx, created.ID, change)
	if !errors.Is(err, mwanachamataskmanager.ErrRepeatingTaskStopped) {
		t.Fatalf("want ErrRepeatingTaskStopped, got %v", err)
	}
	got, err := m.GetRepeatingTask(ctx, created.ID)
	if err != nil || got.Title != "Weekly count" || got.StoppedAt == "" {
		t.Fatalf("the stopped schedule was changed: %+v err %v", got, err)
	}
}

func TestARepeatingTaskUpdateWithAnInvalidScheduleIsRefusedAndChangesNothing(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	created, err := m.StartRepeatingTask(ctx, weeklyCount())
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(*mwanachamataskmanager.NewRepeatingTask){
		"empty rule":    func(n *mwanachamataskmanager.NewRepeatingTask) { n.Schedule.Rule = "BYDAY=" },
		"unknown zone":  func(n *mwanachamataskmanager.NewRepeatingTask) { n.Schedule.Zone = "Mars/Base" },
		"no title":      func(n *mwanachamataskmanager.NewRepeatingTask) { n.Title = "" },
		"agent":         func(n *mwanachamataskmanager.NewRepeatingTask) { n.AssigneeKind = "agent" },
		"open days set": func(n *mwanachamataskmanager.NewRepeatingTask) { n.Schedule.OpenDays = 3 },
	}
	for name, mutate := range bad {
		change := weeklyCount()
		mutate(&change)
		_, err := m.UpdateRepeatingTask(ctx, created.ID, change)
		if !errors.Is(err, mwanachamataskmanager.ErrInvalidRepeatingTask) {
			t.Fatalf("%s: want ErrInvalidRepeatingTask, got %v", name, err)
		}
	}
	got, err := m.GetRepeatingTask(ctx, created.ID)
	if err != nil || got.Title != "Weekly count" || got.ScheduleRule != "FREQ=WEEKLY;BYDAY=MO" {
		t.Fatalf("a refused update changed the record: %+v err %v", got, err)
	}
}

func TestUpdatingAMissingRepeatingTaskIsNotFound(t *testing.T) {
	m := newTestManager(t)
	_, err := m.UpdateRepeatingTask(context.Background(), "nope", weeklyCount())
	if !errors.Is(err, mwanachamataskmanager.ErrRepeatingTaskNotFound) {
		t.Fatalf("want ErrRepeatingTaskNotFound, got %v", err)
	}
}
