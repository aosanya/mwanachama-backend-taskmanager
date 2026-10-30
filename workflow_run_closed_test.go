package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func closeRunAt(t *testing.T, mgr mwanachamataskmanager.TaskManager, code string, minutes int, sweepAfter int) mwanachamataskmanager.WorkflowRun {
	t.Helper()
	ctx := context.Background()
	run, err := mgr.StartWorkflowRun(ctx, "", code, 4, "ask.sent", "customer-1")
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatalf("in_progress: %v", err)
	}
	waiting, err := mgr.BeginWorkflowRunStep(ctx, run.ID, "WI-7", minutes, code+"-cancel")
	if err != nil {
		t.Fatalf("BeginWorkflowRunStep: %v", err)
	}
	if closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(waiting, sweepAfter)); err != nil || len(closed) != 1 {
		t.Fatalf("sweep = %v, %v; want one closed", closed, err)
	}
	got, err := mgr.GetWorkflowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}
	return got
}

func TestListWorkflowRunsClosed_ListsARunClosedPastItsLimitWithItsCancelRun(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	closed := closeRunAt(t, mgr, "ride-ask", 5, 6)

	listed, err := mgr.ListWorkflowRunsClosed(ctx, "")
	if err != nil {
		t.Fatalf("ListWorkflowRunsClosed: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != closed.ID {
		t.Fatalf("closed = %v, want just %s", ids(listed), closed.ID)
	}
	got := listed[0]
	if got.Status != mwanachamataskmanager.WorkflowRunStatusClosed || got.UnderWay {
		t.Errorf("status = %s under_way = %v, want closed and not under way", got.Status, got.UnderWay)
	}
	if got.CancelRunID == "" || got.CompletedAt == "" || got.StepDueAt == "" {
		t.Errorf("run = %+v, want cancel_run_id, completed_at and step_due_at set", got)
	}
	if got.WorkflowCode != "ride-ask" || got.WorkflowVersion != 4 {
		t.Errorf("workflow = %s v%d, want ride-ask v4", got.WorkflowCode, got.WorkflowVersion)
	}
	if _, err := mgr.GetWorkflowRun(ctx, got.CancelRunID); err != nil {
		t.Errorf("cancel run %s: %v", got.CancelRunID, err)
	}
}

func TestListWorkflowRunsClosed_LeavesOutEveryRunThatDidNotClose(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	closed := closeRunAt(t, mgr, "ride-ask", 5, 6)

	underWay, _ := mgr.StartWorkflowRun(ctx, "", "ride-ask", 4, "", "")
	completed, _ := mgr.StartWorkflowRun(ctx, "", "ride-ask", 4, "", "")
	for _, s := range []mwanachamataskmanager.WorkflowRunStatus{mwanachamataskmanager.WorkflowRunStatusInProgress, mwanachamataskmanager.WorkflowRunStatusCompleted} {
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, completed.ID, s, ""); err != nil {
			t.Fatal(err)
		}
	}
	stopped, _ := mgr.StartWorkflowRun(ctx, "", "ride-ask", 4, "", "")
	if _, err := mgr.StopWorkflowRun(ctx, stopped.ID, "manager-1", "", 0); err != nil {
		t.Fatal(err)
	}

	listed, err := mgr.ListWorkflowRunsClosed(ctx, "")
	if err != nil {
		t.Fatalf("ListWorkflowRunsClosed: %v", err)
	}
	if !sameIDs(listed, closed.ID) {
		t.Errorf("closed = %v, want just %s (not %s, %s, %s)", ids(listed), closed.ID, underWay.ID, completed.ID, stopped.ID)
	}
	if now, _ := mgr.ListWorkflowRunsUnderWay(ctx, ""); !sameIDs(now, underWay.ID, closed.CancelRunID) {
		t.Errorf("under way = %v, want %s and the cancel run %s", ids(now), underWay.ID, closed.CancelRunID)
	}
}

func TestListWorkflowRunsClosed_NarrowsToOneWorkflowAndListsTheLatestClosingFirst(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	first := closeRunAt(t, mgr, "ride-ask", 5, 6)
	second := closeRunAt(t, mgr, "ride-ask", 5, 9)
	other := closeRunAt(t, mgr, "buy-now", 5, 6)

	all, err := mgr.ListWorkflowRunsClosed(ctx, "")
	if err != nil || len(all) != 3 {
		t.Fatalf("closed = %v, err = %v; want three", ids(all), err)
	}
	if all[0].ID != second.ID {
		t.Errorf("order = %v, want %s first", ids(all), second.ID)
	}

	rides, err := mgr.ListWorkflowRunsClosed(ctx, "ride-ask")
	if err != nil {
		t.Fatalf("ListWorkflowRunsClosed: %v", err)
	}
	if len(rides) != 2 || rides[0].ID != second.ID || rides[1].ID != first.ID {
		t.Errorf("ride-ask closed = %v, want %s then %s, without %s", ids(rides), second.ID, first.ID, other.ID)
	}
}

func TestListWorkflowRunsClosed_IsEmptyWhenNothingClosed(t *testing.T) {
	mgr := newTestManager(t)
	listed, err := mgr.ListWorkflowRunsClosed(context.Background(), "")
	if err != nil || len(listed) != 0 {
		t.Errorf("closed = %v, err = %v; want none", ids(listed), err)
	}
}
