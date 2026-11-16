package mwanachamataskmanager_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func silentFor(t *testing.T, db *gorm.DB, taskID string, silence time.Duration) {
	t.Helper()
	var table string
	if err := db.Raw(`SELECT m.name FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.type = 'table' AND p.name = 'idle_stopped' AND m.name NOT LIKE '%\_test' ESCAPE '\'`).Scan(&table).Error; err != nil || table == "" {
		t.Fatalf("no task_usage table: %v", err)
	}
	last := time.Now().UTC().Add(-silence).Format("2006-01-02T15:04:05.000000000Z07:00")
	if err := db.Exec("UPDATE "+table+" SET updated_at = ? WHERE id = ?", last, taskID).Error; err != nil {
		t.Fatal(err)
	}
}

func TestClockIdle_NoUsageWriteForTenMinutesStopsTheClockAndLogsWhy(t *testing.T) {
	ctx := context.Background()
	mgr, db, _ := newTestManagerWithDB(t)
	task := workerHolding(t, mgr, "Ready", "hub-steward-swe")
	if _, err := mgr.WriteTaskUsage(ctx, "worker", task.ID, usage(1000, 100, 120)); err != nil {
		t.Fatal(err)
	}
	silentFor(t, db, task.ID, 11*time.Minute)

	reading, err := mgr.GetTaskCaps(ctx, "", task.ID)
	if err != nil {
		t.Fatal(err)
	}

	if reading.Used == nil || !reading.Used.ClockStopped || !reading.Used.IdleStopped {
		t.Fatalf("used %+v", reading.Used)
	}
	notes := logOf(t, mgr, task.ID)
	if len(notes) != 1 || !notes[0].Automatic || !strings.Contains(notes[0].Text, "no usage write for 10 minutes") {
		t.Fatalf("log %+v", notes)
	}
	if got, _ := mgr.GetTask(ctx, task.ID); got.Status != mwanachamataskmanager.TaskStatusInProgress || len(got.Tags) != 2 {
		t.Fatalf("an idle stop must leave the issue as it is: %+v", got)
	}
	wantPlace(t, mgr, task.ID, "doing")
}

func TestClockIdle_AFreshWriteKeepsTheClockRunningAndTheNextWriteRestartsAStoppedOne(t *testing.T) {
	ctx := context.Background()
	mgr, db, _ := newTestManagerWithDB(t)
	task := workerHolding(t, mgr, "Ready")
	if _, err := mgr.WriteTaskUsage(ctx, "worker", task.ID, usage(1, 1, 5)); err != nil {
		t.Fatal(err)
	}
	reading, _ := mgr.GetTaskCaps(ctx, "", task.ID)
	if reading.Used.ClockStopped || len(logOf(t, mgr, task.ID)) != 0 {
		t.Fatalf("a recent write stopped the clock: %+v", reading.Used)
	}

	silentFor(t, db, task.ID, 30*time.Minute)
	if reading, _ = mgr.GetTaskCaps(ctx, "", task.ID); !reading.Used.IdleStopped {
		t.Fatalf("not stopped: %+v", reading.Used)
	}

	restarted, err := mgr.WriteTaskUsage(ctx, "worker", task.ID, usage(2, 2, 6))
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Used.ClockStopped || restarted.Used.IdleStopped {
		t.Fatalf("the next write did not restart the clock: %+v", restarted.Used)
	}
	notes := logOf(t, mgr, task.ID)
	if len(notes) != 2 || !notes[1].Automatic || !strings.Contains(notes[1].Text, "restarted") {
		t.Fatalf("log %+v", notes)
	}
}

func TestClockIdle_AStoppedClockIsNeverSinbinnedAndNearestSaysInterrupted(t *testing.T) {
	ctx := context.Background()
	mgr, db, _ := newTestManagerWithDB(t)
	task := workerHolding(t, mgr, "Ready", "hub-steward-swe")
	if _, err := mgr.WriteTaskUsage(ctx, "worker", task.ID, usage(1, 1, 800)); err != nil {
		t.Fatal(err)
	}
	silentFor(t, db, task.ID, 20*time.Minute)

	item := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{task.ID}})[task.ID]

	if !item.Interrupted {
		t.Fatalf("nearest %+v", item)
	}
	got, _ := mgr.GetTask(ctx, task.ID)
	if got.Status != mwanachamataskmanager.TaskStatusInProgress || len(got.Tags) != 2 {
		t.Fatalf("an interrupted issue was moved: %+v", got)
	}
}
