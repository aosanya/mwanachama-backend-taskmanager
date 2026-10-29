package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestStartWorkflowRun_RefusesAMissingWorkflowOrVersion(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.StartWorkflowRun(ctx, "", "", 1, "", ""); !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
		t.Errorf("no workflow code: err = %v, want ErrInvalidTask", err)
	}
	if _, err := mgr.StartWorkflowRun(ctx, "", "group-order", 0, "", ""); !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
		t.Errorf("version 0: err = %v, want ErrInvalidTask", err)
	}
}

func TestStartWorkflowRun_KeepsTheVersionItBeganOnAcrossEveryStatusChange(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run, err := mgr.StartWorkflowRun(ctx, "", "group-order", 1, "order.sent", "farmer-1")
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	for _, s := range []mwanachamataskmanager.WorkflowRunStatus{
		mwanachamataskmanager.WorkflowRunStatusInProgress,
		mwanachamataskmanager.WorkflowRunStatusPaused,
		mwanachamataskmanager.WorkflowRunStatusInProgress,
		mwanachamataskmanager.WorkflowRunStatusCompleted,
	} {
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, s, ""); err != nil {
			t.Fatalf("→ %s: %v", s, err)
		}
		got, err := mgr.GetWorkflowRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetWorkflowRun: %v", err)
		}
		if got.WorkflowCode != "group-order" || got.WorkflowVersion != 1 {
			t.Fatalf("after %s: workflow = %s v%d, want group-order v1", s, got.WorkflowCode, got.WorkflowVersion)
		}
	}
}

func TestListWorkflowRunsUnderWay_ListsRunsNotEndedWithTheirVersion(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	old, _ := mgr.StartWorkflowRun(ctx, "", "group-order", 1, "", "")
	fresh, _ := mgr.StartWorkflowRun(ctx, "", "group-order", 2, "", "")
	other, _ := mgr.StartWorkflowRun(ctx, "", "buy-now", 1, "", "")
	finished, _ := mgr.StartWorkflowRun(ctx, "", "group-order", 1, "", "")
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, finished.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, finished.ID, mwanachamataskmanager.WorkflowRunStatusCompleted, ""); err != nil {
		t.Fatal(err)
	}

	all, err := mgr.ListWorkflowRunsUnderWay(ctx, "")
	if err != nil {
		t.Fatalf("ListWorkflowRunsUnderWay: %v", err)
	}
	if !sameIDs(all, old.ID, fresh.ID, other.ID) {
		t.Errorf("all under way = %v, want %s %s %s", ids(all), old.ID, fresh.ID, other.ID)
	}

	groupOrders, err := mgr.ListWorkflowRunsUnderWay(ctx, "group-order")
	if err != nil {
		t.Fatalf("ListWorkflowRunsUnderWay: %v", err)
	}
	if !sameIDs(groupOrders, old.ID, fresh.ID) {
		t.Errorf("group-order under way = %v, want %s %s", ids(groupOrders), old.ID, fresh.ID)
	}
	versions := map[string]int{}
	for _, r := range groupOrders {
		versions[r.ID] = r.WorkflowVersion
	}
	if versions[old.ID] != 1 || versions[fresh.ID] != 2 {
		t.Errorf("versions = %v, want old on 1 and fresh on 2", versions)
	}
}

func TestListWorkflowRunsUnderWay_AnOrderRunLeavesTheListWhenItEnds(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	for _, end := range []mwanachamataskmanager.WorkflowRunStatus{
		mwanachamataskmanager.WorkflowRunStatusCompleted,
		mwanachamataskmanager.WorkflowRunStatusFailed,
	} {
		run, _ := mgr.StartWorkflowRun(ctx, "", "buy-now", 1, "", "")
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
			t.Fatal(err)
		}
		listed, _ := mgr.ListWorkflowRunsUnderWay(ctx, "buy-now")
		if !sameIDs(listed, run.ID) {
			t.Fatalf("%s: before ending = %v, want just %s", end, ids(listed), run.ID)
		}
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, end, ""); err != nil {
			t.Fatal(err)
		}
		listed, _ = mgr.ListWorkflowRunsUnderWay(ctx, "buy-now")
		if len(listed) != 0 {
			t.Fatalf("%s: after ending = %v, want none", end, ids(listed))
		}
	}
}

func TestStopWorkflowRun_EndsTheRunAtOnceAndCancelsItsOpenTasks(t *testing.T) {
	mgr, pub := newManagerWithPublisher(t)
	ctx := context.Background()
	run, _ := mgr.StartWorkflowRun(ctx, "", "group-order", 2, "", "")
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatal(err)
	}
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "collect the shares", WorkflowRunID: run.ID})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	stop, err := mgr.StopWorkflowRun(ctx, run.ID, "manager-1", "wrong price list", 0)
	if err != nil {
		t.Fatalf("StopWorkflowRun: %v", err)
	}
	if stop.Stopped.Status != mwanachamataskmanager.WorkflowRunStatusCancelled || stop.Stopped.CompletedAt == "" {
		t.Errorf("status = %s completed_at = %q, want cancelled and set", stop.Stopped.Status, stop.Stopped.CompletedAt)
	}
	if stop.Stopped.CancelledBy != "manager-1" || stop.Stopped.CancelReason != "wrong price list" {
		t.Errorf("stopped by %q because %q", stop.Stopped.CancelledBy, stop.Stopped.CancelReason)
	}
	if stop.Restarted != nil {
		t.Errorf("restarted = %+v, want none", stop.Restarted)
	}
	got, _ := mgr.GetTask(ctx, task.ID)
	if got.Status != mwanachamataskmanager.TaskStatusCancelled {
		t.Errorf("task status = %s, want cancelled", got.Status)
	}
	if listed, _ := mgr.ListWorkflowRunsUnderWay(ctx, "group-order"); len(listed) != 0 {
		t.Errorf("under way after stop = %v, want none", ids(listed))
	}
	if !contains(pub.topicList(), mwanachamataskmanager.TopicRunCancelled) {
		t.Errorf("topics = %v, want work.run.cancelled", pub.topicList())
	}
}

func TestStopWorkflowRun_StopsARunThatHasNotStartedOrIsPaused(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	pending, _ := mgr.StartWorkflowRun(ctx, "", "buy-now", 1, "", "")
	paused, _ := mgr.StartWorkflowRun(ctx, "", "buy-now", 1, "", "")
	for _, s := range []mwanachamataskmanager.WorkflowRunStatus{mwanachamataskmanager.WorkflowRunStatusInProgress, mwanachamataskmanager.WorkflowRunStatusPaused} {
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, paused.ID, s, ""); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{pending.ID, paused.ID} {
		stop, err := mgr.StopWorkflowRun(ctx, id, "manager-1", "", 0)
		if err != nil {
			t.Fatalf("StopWorkflowRun %s: %v", id, err)
		}
		if stop.Stopped.Status != mwanachamataskmanager.WorkflowRunStatusCancelled {
			t.Errorf("%s: status = %s, want cancelled", id, stop.Stopped.Status)
		}
	}
}

func TestStopWorkflowRun_RefusesARunThatHasEnded(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := createRunAtStatus(t, mgr, mwanachamataskmanager.WorkflowRunStatusCompleted)
	if _, err := mgr.StopWorkflowRun(ctx, run.ID, "manager-1", "", 0); !errors.Is(err, mwanachamataskmanager.ErrCannotCancelTerminalRun) {
		t.Errorf("err = %v, want ErrCannotCancelTerminalRun", err)
	}
	if _, err := mgr.StopWorkflowRun(ctx, "no-such-run", "manager-1", "", 0); !errors.Is(err, mwanachamataskmanager.ErrWorkflowRunNotFound) {
		t.Errorf("unknown run: err = %v, want ErrWorkflowRunNotFound", err)
	}
}

func TestStopWorkflowRun_StartsAgainOnTheGoodVersion(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	bad, _ := mgr.StartWorkflowRun(ctx, "", "group-order", 2, "order.sent", "farmer-1")
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, bad.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatal(err)
	}
	staying, _ := mgr.StartWorkflowRun(ctx, "", "group-order", 2, "", "")

	stop, err := mgr.StopWorkflowRun(ctx, bad.ID, "manager-1", "back to version 1", 1)
	if err != nil {
		t.Fatalf("StopWorkflowRun: %v", err)
	}
	if stop.Restarted == nil {
		t.Fatal("restarted = nil, want a run on version 1")
	}
	r := stop.Restarted
	if r.WorkflowCode != "group-order" || r.WorkflowVersion != 1 || r.RestartedFromRunID != bad.ID {
		t.Errorf("restarted = %s v%d from %q, want group-order v1 from %s", r.WorkflowCode, r.WorkflowVersion, r.RestartedFromRunID, bad.ID)
	}
	if r.TriggerEvent != "order.sent" || r.Initiator != "farmer-1" || r.ID == bad.ID || r.Status != mwanachamataskmanager.WorkflowRunStatusPending {
		t.Errorf("restarted = %+v, want same trigger and initiator on a fresh pending run", r)
	}
	listed, _ := mgr.ListWorkflowRunsUnderWay(ctx, "group-order")
	if !sameIDs(listed, r.ID, staying.ID) {
		t.Errorf("under way = %v, want the restarted run and the untouched one", ids(listed))
	}
}

func TestStopWorkflowRun_NoWorkflowToStartAgainStopsNothing(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run, _ := mgr.CreateWorkflowRun(ctx, "", "", "")
	if _, err := mgr.StopWorkflowRun(ctx, run.ID, "manager-1", "", 1); !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
		t.Fatalf("err = %v, want ErrInvalidTask", err)
	}
	got, _ := mgr.GetWorkflowRun(ctx, run.ID)
	if got.Status != mwanachamataskmanager.WorkflowRunStatusPending {
		t.Errorf("status = %s, want pending: a refused stop must stop nothing", got.Status)
	}
	if _, err := mgr.StopWorkflowRun(ctx, run.ID, "manager-1", "", -1); !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
		t.Errorf("negative version: err = %v, want ErrInvalidTask", err)
	}
}

func TestUnderWayIndex_IsOneIndexOnlyTheRunsStillGoing(t *testing.T) {
	_, db, tables := newTestManagerWithDB(t, nil)
	var defs []string
	if err := db.Raw(`select sql from sqlite_master where type='index' and tbl_name = ? and sql like '%under_way%'`, tables.WorkflowRuns).Scan(&defs).Error; err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 {
		t.Fatalf("under_way indexes = %d, want 1: %v", len(defs), defs)
	}
	if !strings.Contains(defs[0], "workflow_code") || !strings.Contains(defs[0], "workflow_version") || !strings.Contains(strings.ToLower(defs[0]), "where") {
		t.Errorf("index = %s, want workflow_code and workflow_version, partial", defs[0])
	}
}

func ids(runs []mwanachamataskmanager.WorkflowRun) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.ID
	}
	return out
}

func sameIDs(runs []mwanachamataskmanager.WorkflowRun, want ...string) bool {
	if len(runs) != len(want) {
		return false
	}
	have := map[string]bool{}
	for _, r := range runs {
		have[r.ID] = true
	}
	for _, w := range want {
		if !have[w] {
			return false
		}
	}
	return true
}

func TestUnderWayIndex_NameFitsUnderAnAgencyInstanceIDAndItsTestTwin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	agencySpec, _ := specForInstance(t, "spec/examples/work.taskmanager.json", "agyd1662f9a")
	if err := spec.Migrate(db, agencySpec); err != nil {
		t.Fatalf("Migrate under a real agency instance id: %v", err)
	}
}
