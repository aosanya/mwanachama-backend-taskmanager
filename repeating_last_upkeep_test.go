package mwanachamataskmanager_test

import (
	"context"
	"github.com/aosanya/mwanachama-backend-shared/recur"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
	"testing"
	"time"
)

var (
	firstWeek  = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	secondWeek = time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
)

func startWeeklySchedule(t *testing.T, mgr mwanachamataskmanager.TaskManager, title, actor string) mwanachamataskmanager.RepeatingTaskRecord {
	t.Helper()
	started, err := mgr.StartRepeatingTask(context.Background(), mwanachamataskmanager.NewRepeatingTask{Title: title, Schedule: recur.Wire{Start: "2026-11-02T07:00:00Z", Zone: budapest, Rule: "FREQ=WEEKLY;BYDAY=MO"}, AssigneeKind: models.AssigneeActor, AssigneeID: actor})
	if err != nil {
		t.Fatalf("StartRepeatingTask %q: %v", title, err)
	}
	return started
}
func makeJobDue(t *testing.T, mgr mwanachamataskmanager.TaskManager, scheduleID string, at time.Time) mwanachamataskmanager.RepeatingTaskPeriod {
	t.Helper()
	period, made, err := mgr.MakeRepeatingTaskDue(context.Background(), scheduleID, at)
	if err != nil {
		t.Fatalf("MakeRepeatingTaskDue at %s: %v", at, err)
	}
	if !made || period.TaskID == "" || period.DueAt == "" {
		t.Fatalf("MakeRepeatingTaskDue at %s made nothing: %+v made=%v", at, period, made)
	}
	return period
}
func finishJob(t *testing.T, mgr mwanachamataskmanager.TaskManager, taskID string) models.Task {
	t.Helper()
	ctx := context.Background()
	task, err := mgr.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask %s: %v", taskID, err)
	}
	for _, next := range []models.TaskStatus{models.TaskStatusInProgress, models.TaskStatusCompleted} {
		task.Status = next
		if task, err = mgr.UpdateTask(ctx, task); err != nil {
			t.Fatalf("UpdateTask to %s: %v", next, err)
		}
	}
	done, err := mgr.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask %s after finishing: %v", taskID, err)
	}
	if done.CompletedAt == "" {
		t.Fatalf("the manager left completed_at empty on a finished job")
	}
	return done
}

type lastUpkeep struct {
	taskID string
	dueAt  string
	doneAt string
}

func assertLastUpkeep(t *testing.T, label string, got mwanachamataskmanager.RepeatingTaskRecord, want lastUpkeep) {
	t.Helper()
	if got.LastTaskID != want.taskID {
		t.Errorf("%s: last_task_id = %q, want %q", label, got.LastTaskID, want.taskID)
	}
	if got.LastDueAt != want.dueAt {
		t.Errorf("%s: last_due_at = %q, want %q", label, got.LastDueAt, want.dueAt)
	}
	if got.LastDoneAt != want.doneAt {
		t.Errorf("%s: last_done_at = %q, want %q", label, got.LastDoneAt, want.doneAt)
	}
}
func assertBothReads(t *testing.T, mgr mwanachamataskmanager.TaskManager, scheduleID string, want lastUpkeep) {
	t.Helper()
	ctx := context.Background()
	read, err := mgr.GetRepeatingTask(ctx, scheduleID)
	if err != nil {
		t.Fatalf("GetRepeatingTask: %v", err)
	}
	assertLastUpkeep(t, "get", read, want)
	listed, err := mgr.ListRepeatingTasks(ctx, mwanachamataskmanager.RepeatingTaskFilter{})
	if err != nil {
		t.Fatalf("ListRepeatingTasks: %v", err)
	}
	for _, rec := range listed {
		if rec.ID == scheduleID {
			assertLastUpkeep(t, "list", rec, want)
			return
		}
	}
	t.Fatalf("ListRepeatingTasks did not return schedule %s", scheduleID)
}
func TestLastUpkeepIsEmptyForAScheduleThatMadeNoJob(t *testing.T) {
	mgr := newTestManager(t)
	schedule := startWeeklySchedule(t, mgr, "Inspect the playground", "actor-1")
	assertBothReads(t, mgr, schedule.ID, lastUpkeep{})
}
func TestLastUpkeepNamesTheOpenJobOfAScheduleWithOneJob(t *testing.T) {
	mgr := newTestManager(t)
	schedule := startWeeklySchedule(t, mgr, "Inspect the playground", "actor-1")
	period := makeJobDue(t, mgr, schedule.ID, firstWeek)
	assertBothReads(t, mgr, schedule.ID, lastUpkeep{taskID: period.TaskID, dueAt: period.DueAt})
}
func TestLastUpkeepFollowsTheNewestJobWhileTheOlderIsFinished(t *testing.T) {
	mgr := newTestManager(t)
	schedule := startWeeklySchedule(t, mgr, "Inspect the playground", "actor-1")
	older := makeJobDue(t, mgr, schedule.ID, firstWeek)
	newer := makeJobDue(t, mgr, schedule.ID, secondWeek)
	if older.DueAt >= newer.DueAt {
		t.Fatalf("the second week's job is due %s, not after the first week's %s", newer.DueAt, older.DueAt)
	}
	finishJob(t, mgr, older.TaskID)
	assertBothReads(t, mgr, schedule.ID, lastUpkeep{taskID: newer.TaskID, dueAt: newer.DueAt})
}
func TestLastUpkeepCarriesWhenTheNewestJobWasFinished(t *testing.T) {
	mgr := newTestManager(t)
	schedule := startWeeklySchedule(t, mgr, "Inspect the playground", "actor-1")
	older := makeJobDue(t, mgr, schedule.ID, firstWeek)
	newer := makeJobDue(t, mgr, schedule.ID, secondWeek)
	finishJob(t, mgr, older.TaskID)
	done := finishJob(t, mgr, newer.TaskID)
	assertBothReads(t, mgr, schedule.ID, lastUpkeep{taskID: newer.TaskID, dueAt: newer.DueAt, doneAt: done.CompletedAt})
}
func TestLastUpkeepOfTwoSchedulesInOneListDoesNotCross(t *testing.T) {
	mgr := newTestManager(t)
	playground := startWeeklySchedule(t, mgr, "Inspect the playground", "actor-1")
	pumps := startWeeklySchedule(t, mgr, "Grease the pumps", "actor-2")
	playgroundJob := makeJobDue(t, mgr, playground.ID, firstWeek)
	pumpsOlder := makeJobDue(t, mgr, pumps.ID, firstWeek)
	pumpsNewer := makeJobDue(t, mgr, pumps.ID, secondWeek)
	playgroundDone := finishJob(t, mgr, playgroundJob.TaskID)
	finishJob(t, mgr, pumpsOlder.TaskID)
	assertBothReads(t, mgr, playground.ID, lastUpkeep{taskID: playgroundJob.TaskID, dueAt: playgroundJob.DueAt, doneAt: playgroundDone.CompletedAt})
	assertBothReads(t, mgr, pumps.ID, lastUpkeep{taskID: pumpsNewer.TaskID, dueAt: pumpsNewer.DueAt})
}
