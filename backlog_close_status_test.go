package mwanachamataskmanager_test

import (
	"context"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"testing"
)

func closingBacklog(t *testing.T, mgr mwanachamataskmanager.TaskManager) string {
	t.Helper()
	created, err := mgr.CreateBacklog(context.Background(), "Work", []string{"To do", "In progress", "Review", "Done", "Archive", "Cancelled"})
	if err != nil {
		t.Fatal(err)
	}
	return created.ID
}
func inStep(t *testing.T, mgr mwanachamataskmanager.TaskManager, backlog, step, title string) mwanachamataskmanager.Task {
	t.Helper()
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: title})
	ctx := context.Background()
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, backlog, "to-do"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToState(ctx, task.ID, "Doing"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, backlog, step); err != nil {
		t.Fatalf("move %s to %s: %v", title, step, err)
	}
	got, err := mgr.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
func blocksStart(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) bool {
	t.Helper()
	deps, err := mgr.TaskDependenciesFor(context.Background(), "", id)
	if err != nil {
		t.Fatal(err)
	}
	return deps.BlocksStart
}
func statusOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) mwanachamataskmanager.TaskStatus {
	t.Helper()
	got, err := mgr.GetTask(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return got.Status
}
func TestClosingMove_DoneStepCompletesAnIssueInProgress(t *testing.T) {
	mgr := newTestManager(t)
	backlog := closingBacklog(t, mgr)
	task := inStep(t, mgr, backlog, "review", "built")
	if task.Status != mwanachamataskmanager.TaskStatusInProgress {
		t.Fatalf("review keeps %s, want in_progress", task.Status)
	}
	if _, err := mgr.MoveTaskToStep(context.Background(), task.ID, backlog, "done"); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, mgr, task.ID); got != mwanachamataskmanager.TaskStatusCompleted {
		t.Fatalf("status %s after Done, want completed", got)
	}
}
func TestClosingMove_ArchiveStepCompletesAndCancelledStepCancels(t *testing.T) {
	mgr := newTestManager(t)
	backlog := closingBacklog(t, mgr)
	archived := inStep(t, mgr, backlog, "review", "old")
	dropped := inStep(t, mgr, backlog, "in-progress", "dropped")
	if _, err := mgr.MoveTaskToStep(context.Background(), archived.ID, backlog, "archive"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(context.Background(), dropped.ID, backlog, "cancelled"); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, mgr, archived.ID); got != mwanachamataskmanager.TaskStatusCompleted {
		t.Fatalf("archived status %s, want completed", got)
	}
	if got := statusOf(t, mgr, dropped.ID); got != mwanachamataskmanager.TaskStatusCancelled {
		t.Fatalf("cancelled status %s, want cancelled", got)
	}
}
func TestClosingMove_DependentCanStartOnceItsGateIsMovedToDone(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	backlog := closingBacklog(t, mgr)
	gate := inStep(t, mgr, backlog, "review", "gate")
	waits := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "waits"})
	if _, err := mgr.MoveTaskToStep(ctx, waits.ID, backlog, "to-do"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{Label: mwanachamataskmanager.RelLabelDependsOn, FromID: waits.ID, ToID: gate.ID}); err != nil {
		t.Fatal(err)
	}
	if open := blocksStart(t, mgr, waits.ID); !open {
		t.Fatal("the dependent should wait while its gate is in Review")
	}
	if _, err := mgr.MoveTaskToStep(ctx, gate.ID, backlog, "done"); err != nil {
		t.Fatal(err)
	}
	if open := blocksStart(t, mgr, waits.ID); open {
		t.Fatal("the dependent still waits after its gate was moved to Done")
	}
}
func TestClosingMove_MovingOutOfDoneReopensTheIssue(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	backlog := closingBacklog(t, mgr)
	task := inStep(t, mgr, backlog, "review", "again")
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, backlog, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, backlog, "in-progress"); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, mgr, task.ID); got != mwanachamataskmanager.TaskStatusInProgress {
		t.Fatalf("status %s after leaving Done for In progress, want in_progress", got)
	}
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, backlog, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, backlog, "to-do"); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, mgr, task.ID); got != mwanachamataskmanager.TaskStatusPending {
		t.Fatalf("status %s after leaving Done for To do, want pending", got)
	}
}
func TestClosingMove_ReviewAndOpenStepsLeaveTheStatusAlone(t *testing.T) {
	mgr := newTestManager(t)
	backlog := closingBacklog(t, mgr)
	task := inStep(t, mgr, backlog, "review", "mid")
	if got := statusOf(t, mgr, task.ID); got != mwanachamataskmanager.TaskStatusInProgress {
		t.Fatalf("status %s in Review, want in_progress", got)
	}
	if _, err := mgr.MoveTaskToStep(context.Background(), task.ID, backlog, "to-do"); err != nil {
		t.Fatal(err)
	}
	if got := statusOf(t, mgr, task.ID); got != mwanachamataskmanager.TaskStatusInProgress {
		t.Fatalf("status %s after a move to To do, want in_progress unchanged", got)
	}
}
