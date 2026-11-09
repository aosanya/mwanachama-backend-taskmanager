package mwanachamataskmanager_test

import (
	"context"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func placedAt(t *testing.T, mgr mwanachamataskmanager.TaskManager, title, step string) mwanachamataskmanager.Task {
	t.Helper()
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: title})
	if _, err := mgr.MoveTaskToStep(context.Background(), task.ID, "backlog", step); err != nil {
		t.Fatalf("MoveTaskToStep %s: %v", step, err)
	}
	return task
}

func wantPlace(t *testing.T, mgr mwanachamataskmanager.TaskManager, id, wantStep string) {
	t.Helper()
	if backlog, step := placeOf(t, mgr, id); backlog != "backlog" || step != wantStep {
		t.Fatalf("got %q at %q, want backlog at %q", backlog, step, wantStep)
	}
}

func assignedAndWaiting(t *testing.T, mgr mwanachamataskmanager.TaskManager) (waiting, gate mwanachamataskmanager.Task) {
	t.Helper()
	ctx := context.Background()
	if _, err := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "worker"}); err != nil {
		t.Fatal(err)
	}
	gate = createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "gate"})
	waiting = placedAt(t, mgr, "waits", "to-do")
	if _, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelDependsOn, FromID: waiting.ID, ToID: gate.ID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AssignTask(ctx, waiting.ID, "worker", ""); err != nil {
		t.Fatal(err)
	}
	return waiting, gate
}

func TestStatusPaths_AssigningATaskThatWaitsOnAnotherMovesItToTheWaitingStep(t *testing.T) {
	mgr := newTestManager(t)
	waiting, _ := assignedAndWaiting(t, mgr)
	got, err := mgr.GetTask(context.Background(), waiting.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != mwanachamataskmanager.TaskStatusBlocked {
		t.Fatalf("status %s, want blocked", got.Status)
	}
	wantPlace(t, mgr, waiting.ID, "waiting")
}

func TestStatusPaths_UnblockingADependentMovesItBackToTheToDoStep(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	waiting, gate := assignedAndWaiting(t, mgr)
	if _, err := mgr.MoveTaskToStep(ctx, waiting.ID, "backlog", "waiting"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToState(ctx, gate.ID, "Done"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.UnblockDependents(ctx, gate.ID); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(ctx, waiting.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != mwanachamataskmanager.TaskStatusPending {
		t.Fatalf("status %s, want pending", got.Status)
	}
	wantPlace(t, mgr, waiting.ID, "to-do")
}

func startedRunWith(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) mwanachamataskmanager.WorkflowRun {
	t.Helper()
	ctx := context.Background()
	run, err := mgr.CreateWorkflowRun(ctx, "run", "test", "tester")
	if err != nil {
		t.Fatal(err)
	}
	if run, err = mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatal(err)
	}
	if err := mgr.LinkTaskToRun(ctx, run.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	return run
}

func TestStatusPaths_CancellingARunMovesItsTasksToTheCancelledStep(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := placedAt(t, mgr, "in a run", "doing")
	run := startedRunWith(t, mgr, task)
	if _, err := mgr.CancelWorkflowRun(ctx, run.ID, "stop", "tester", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != mwanachamataskmanager.TaskStatusCancelled {
		t.Fatalf("status %s, want cancelled", got.Status)
	}
	wantPlace(t, mgr, task.ID, "cancelled")
}

func TestStatusPaths_CancellingACaseMovesItsTasksToTheCancelledStep(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := placedAt(t, mgr, "in a case", "doing")
	opened, err := mgr.OpenCase(ctx, "requester", mwanachamataskmanager.Case{SubjectKind: "thing", SubjectID: "1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.LinkCaseTask(ctx, opened.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.EndCase(ctx, opened.ID, mwanachamataskmanager.CaseStatusCancelled, "no longer needed"); err != nil {
		t.Fatal(err)
	}
	wantPlace(t, mgr, task.ID, "cancelled")
}

func TestStatusPaths_RollingBackARunResetsItsTasksToTheToDoStep(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := placedAt(t, mgr, "rolled back", "doing")
	run := startedRunWith(t, mgr, task)
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusCompleted, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.RollbackWorkflowRun(ctx, run.ID, "undo"); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != mwanachamataskmanager.TaskStatusPending {
		t.Fatalf("status %s, want pending", got.Status)
	}
	wantPlace(t, mgr, task.ID, "to-do")
}
