package mwanachamataskmanager_test

import (
	"context"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/recur"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func storedMonthlyReview(t *testing.T) mwanachamataskmanager.RepeatingTask {
	t.Helper()
	rule, err := recur.ParseRule("FREQ=MONTHLY")
	if err != nil {
		t.Fatal(err)
	}
	nairobi, err := time.LoadLocation("Africa/Nairobi")
	if err != nil {
		t.Fatal(err)
	}
	return mwanachamataskmanager.RepeatingTask{
		ID:           "ignored",
		Title:        "Margin review",
		Schedule:     recur.Schedule{Start: time.Date(2026, 1, 1, 9, 0, 0, 0, nairobi), Zone: nairobi, Rule: &rule},
		AssigneeKind: models.AssigneeActor,
		AssigneeID:   "actor-7",
	}
}

func TestARepeatingTaskIsStoredAndReadBackWithItsSchedule(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	rec, err := m.CreateRepeatingTask(ctx, storedMonthlyReview(t))
	if err != nil {
		t.Fatal(err)
	}
	if rec.ID == "" || rec.ID == "ignored" {
		t.Fatalf("id %q was not minted by the store", rec.ID)
	}
	got, err := m.GetRepeatingTask(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := mwanachamataskmanager.RepeatingTaskFromRecord(got)
	if err != nil {
		t.Fatal(err)
	}
	if r.Schedule.Zone.String() != "Africa/Nairobi" || r.Schedule.Rule.String() != "FREQ=MONTHLY" || !r.Schedule.Start.Equal(time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)) {
		t.Fatalf("schedule read back as %+v", r.Schedule)
	}
	list, err := m.ListRepeatingTasks(ctx, mwanachamataskmanager.RepeatingTaskFilter{AssigneeID: "actor-7"})
	if err != nil || len(list) != 1 {
		t.Fatalf("list %v err %v", list, err)
	}
}

func TestARepeatingTaskIsRefusedWithoutAValidSchedule(t *testing.T) {
	m := newTestManager(t)
	r := storedMonthlyReview(t)
	r.Schedule.Zone = nil
	if _, err := m.CreateRepeatingTask(context.Background(), r); err == nil {
		t.Fatal("a schedule with no zone was stored")
	}
}

func TestAPeriodMakesOneAssignedTaskHoweverOftenItIsTicked(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	rec, err := m.CreateRepeatingTask(ctx, storedMonthlyReview(t))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	first, made, err := m.MakeRepeatingTaskDue(ctx, rec.ID, at)
	if err != nil || !made {
		t.Fatalf("made=%v err=%v", made, err)
	}
	again, made, err := m.MakeRepeatingTaskDue(ctx, rec.ID, at.Add(24*time.Hour))
	if err != nil || made || again.TaskID != first.TaskID {
		t.Fatalf("second tick made=%v task %q vs %q err=%v", made, again.TaskID, first.TaskID, err)
	}
	task, err := m.GetTask(ctx, first.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Margin review" || first.DueAt != "2026-04-01T06:00:00Z" {
		t.Fatalf("task %+v due %s", task, first.DueAt)
	}
	held, err := m.ListHeldTasks(ctx, models.HolderTasksQuery{ActorID: "actor-7"})
	if err != nil || len(held) != 1 || held[0].ID != first.TaskID {
		t.Fatalf("held %v err %v", held, err)
	}
}

func TestAStoppedRepeatingTaskMakesNothingAndKeepsWhatItMade(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	rec, err := m.CreateRepeatingTask(ctx, storedMonthlyReview(t))
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := m.MakeRepeatingTaskDue(ctx, rec.ID, time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := m.StopRepeatingTask(ctx, rec.ID)
	if err != nil || stopped.StoppedAt == "" {
		t.Fatalf("stopped %+v err %v", stopped, err)
	}
	if _, made, err := m.MakeRepeatingTaskDue(ctx, rec.ID, time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)); err != nil || made {
		t.Fatalf("a stopped template made a task: made=%v err=%v", made, err)
	}
	if _, err := m.GetTask(ctx, first.TaskID); err != nil {
		t.Fatalf("the past task went with the stop: %v", err)
	}
}
