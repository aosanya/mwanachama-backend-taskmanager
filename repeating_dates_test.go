package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/recur"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"gorm.io/gorm"
)

func termReports(t *testing.T, end time.Time) mwanachamataskmanager.RepeatingTask {
	t.Helper()
	var dates []recur.Date
	for _, s := range []string{"2026-01-05", "2026-04-20", "2026-09-07"} {
		d, err := recur.ParseDate(s)
		if err != nil {
			t.Fatal(err)
		}
		dates = append(dates, d)
	}
	return mwanachamataskmanager.RepeatingTask{
		ID:           "ignored",
		Title:        "Term report",
		Schedule:     recur.Schedule{Start: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), End: end, Zone: time.UTC, Dates: dates},
		AssigneeKind: models.AssigneeActor,
		AssigneeID:   "actor-7",
	}
}

func TestARepeatingTaskOnAListOfDatesIsRefusedWithoutAnEnd(t *testing.T) {
	m := newTestManager(t)
	_, err := m.CreateRepeatingTask(context.Background(), termReports(t, time.Time{}))
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRepeatingTask) {
		t.Fatalf("err %v", err)
	}
}

func TestTheLastDatesTaskIsMadeAndDueOnTheEnd(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	end := time.Date(2026, 12, 18, 0, 0, 0, 0, time.UTC)
	rec, err := m.CreateRepeatingTask(ctx, termReports(t, end))
	if err != nil {
		t.Fatal(err)
	}
	period, made, err := m.MakeRepeatingTaskDue(ctx, rec.ID, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || !made {
		t.Fatalf("made=%v err=%v", made, err)
	}
	if period.PeriodStart != "2026-09-07T00:00:00Z" || period.DueAt != "2026-12-18T00:00:00Z" {
		t.Fatalf("period %+v", period)
	}
	task, err := m.GetTask(ctx, period.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	due, err := time.Parse(time.RFC3339Nano, task.DueAt)
	if err != nil || !due.Equal(end) {
		t.Fatalf("due %q err %v", task.DueAt, err)
	}
}

func TestTheSweepReadsEveryRunningRepeatingTaskAndNoStoppedOne(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	const total = 503
	var stoppedID string
	for i := 0; i < total; i++ {
		rec, err := m.CreateRepeatingTask(ctx, storedMonthlyReview(t))
		if err != nil {
			t.Fatal(err)
		}
		if i == 7 {
			stoppedID = rec.ID
		}
	}
	if _, err := m.StopRepeatingTask(ctx, stoppedID); err != nil {
		t.Fatal(err)
	}
	running, err := m.ListRunningRepeatingTasks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(running) != total-1 {
		t.Fatalf("running %d, want %d", len(running), total-1)
	}
	for _, r := range running {
		if r.ID == stoppedID || r.StoppedAt != "" {
			t.Fatalf("a stopped template was swept: %+v", r)
		}
	}
}

func TestATickThatLosesTheRaceForAPeriodAnswersAlreadyMade(t *testing.T) {
	m, db, _ := newTestManagerWithDB(t, nil)
	ctx := context.Background()
	rec, err := m.CreateRepeatingTask(ctx, storedMonthlyReview(t))
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	var rival models.RepeatingTaskPeriod
	won := false
	if err := db.Callback().Query().After("gorm:query").Register("rival_tick", func(tx *gorm.DB) {
		if won || !strings.Contains(tx.Statement.SQL.String(), "period_key") {
			return
		}
		won = true
		var made bool
		var err error
		rival, made, err = m.MakeRepeatingTaskDue(context.Background(), rec.ID, at)
		if err != nil || !made {
			t.Errorf("rival made=%v err=%v", made, err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	period, made, err := m.MakeRepeatingTaskDue(ctx, rec.ID, at)
	if err != nil || made {
		t.Fatalf("made=%v err=%v", made, err)
	}
	if period.PeriodKey != rival.PeriodKey || period.TaskID != rival.TaskID {
		t.Fatalf("period %+v, rival %+v", period, rival)
	}
	held, err := m.ListHeldTasks(ctx, models.HolderTasksQuery{ActorID: "actor-7"})
	if err != nil || len(held) != 1 {
		t.Fatalf("held %v err %v", held, err)
	}
}
