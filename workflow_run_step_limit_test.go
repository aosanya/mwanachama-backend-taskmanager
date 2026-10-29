package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func minutesAfter(run mwanachamataskmanager.WorkflowRun, minutes int) string {
	started, _ := time.Parse(time.RFC3339, run.CurrentStepStartedAt)
	return started.Add(time.Duration(minutes) * time.Minute).UTC().Format(time.RFC3339)
}

func waitingRun(t *testing.T, mgr mwanachamataskmanager.TaskManager, limit int) mwanachamataskmanager.WorkflowRun {
	t.Helper()
	ctx := context.Background()
	run, err := mgr.StartWorkflowRun(ctx, "", "ride-ask", 3, "ask.sent", "customer-1")
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatalf("in_progress: %v", err)
	}
	waiting, err := mgr.BeginWorkflowRunStep(ctx, run.ID, "WI-7", limit, "ride-ask-cancel")
	if err != nil {
		t.Fatalf("BeginWorkflowRunStep: %v", err)
	}
	return waiting
}

func TestBeginWorkflowRunStep_RefusesWhatCannotBeKept(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run, _ := mgr.StartWorkflowRun(ctx, "", "ride-ask", 1, "", "")
	cases := []struct {
		name   string
		step   string
		limit  int
		cancel string
	}{
		{"no step", "", 5, "cancel"},
		{"a negative limit", "WI-7", -1, "cancel"},
		{"a limit with no cancel workflow", "WI-7", 5, ""},
		{"a cancel workflow with no limit", "WI-7", 0, "cancel"},
	}
	for _, c := range cases {
		if _, err := mgr.BeginWorkflowRunStep(ctx, run.ID, c.step, c.limit, c.cancel); !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
			t.Errorf("%s: err = %v, want ErrInvalidTask", c.name, err)
		}
	}
	if _, err := mgr.BeginWorkflowRunStep(ctx, "no-such-run", "WI-7", 5, "cancel"); !errors.Is(err, mwanachamataskmanager.ErrWorkflowRunNotFound) {
		t.Errorf("missing run: err = %v, want ErrWorkflowRunNotFound", err)
	}
}

func TestBeginWorkflowRunStep_RefusesARunThatHasEnded(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusCompleted, ""); err != nil {
		t.Fatalf("completed: %v", err)
	}
	if _, err := mgr.BeginWorkflowRunStep(ctx, run.ID, "WI-8", 5, "cancel"); !errors.Is(err, mwanachamataskmanager.ErrCannotCancelTerminalRun) {
		t.Errorf("err = %v, want ErrCannotCancelTerminalRun", err)
	}
}

func TestBeginWorkflowRunStep_WritesTheDueTimeTheLimitReaches(t *testing.T) {
	mgr := newTestManager(t)
	run := waitingRun(t, mgr, 15)
	if run.CurrentStepID != "WI-7" || run.StepCancelWorkflowCode != "ride-ask-cancel" {
		t.Fatalf("step = %q cancel = %q", run.CurrentStepID, run.StepCancelWorkflowCode)
	}
	if run.StepDueAt != minutesAfter(run, 15) {
		t.Errorf("step_due_at = %s, want 15 minutes after %s", run.StepDueAt, run.CurrentStepStartedAt)
	}
	got, _ := mgr.GetWorkflowRun(context.Background(), run.ID)
	if got.StepDueAt != run.StepDueAt || got.WorkflowCode != "ride-ask" || got.WorkflowVersion != 3 {
		t.Errorf("reread = %+v", got)
	}
}

func TestCloseWorkflowRunsPastLimit_LeavesAStepStillInsideItsLimit(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(run, 4))
	if err != nil || len(closed) != 0 {
		t.Fatalf("closed = %v, err = %v; want none", closed, err)
	}
	got, _ := mgr.GetWorkflowRun(ctx, run.ID)
	if got.Status != mwanachamataskmanager.WorkflowRunStatusInProgress || got.UnderWay != true {
		t.Errorf("status = %s under_way = %v, want in_progress and under way", got.Status, got.UnderWay)
	}
}

func TestCloseWorkflowRunsPastLimit_ClosesTheRunAndOpensItsCancelWorkflow(t *testing.T) {
	mgr, pub := newManagerWithPublisher(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "ask the rider", WorkflowRunID: run.ID})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(run, 6))
	if err != nil || len(closed) != 1 {
		t.Fatalf("closed = %v, err = %v; want one", closed, err)
	}
	got := closed[0]
	if got.ID != run.ID || got.Status != mwanachamataskmanager.WorkflowRunStatusClosed || got.UnderWay {
		t.Fatalf("closed run = %+v", got)
	}
	if got.CancelledBy != "watchdog" || got.CompletedAt == "" || got.StepDueAt != run.StepDueAt || got.CurrentStepID != "WI-7" {
		t.Errorf("closed run keeps = %+v", got)
	}

	cancelRun, err := mgr.GetWorkflowRun(ctx, got.CancelRunID)
	if err != nil {
		t.Fatalf("cancel run: %v", err)
	}
	if cancelRun.WorkflowCode != "ride-ask-cancel" || cancelRun.WorkflowVersion != 3 || !cancelRun.UnderWay {
		t.Errorf("cancel run = %+v, want ride-ask-cancel on version 3 and under way", cancelRun)
	}
	if cancelRun.TriggerEvent != "step_limit_passed:"+run.ID || cancelRun.Initiator != "watchdog" {
		t.Errorf("cancel run trigger = %q initiator = %q", cancelRun.TriggerEvent, cancelRun.Initiator)
	}

	gotTask, _ := mgr.GetTask(ctx, task.ID)
	if gotTask.Status != mwanachamataskmanager.TaskStatusCancelled {
		t.Errorf("task status = %s, want cancelled", gotTask.Status)
	}
	event, ok := findEvent(pub.full, mwanachamataskmanager.TopicRunClosed)
	if !ok {
		t.Fatalf("no %s event in %v", mwanachamataskmanager.TopicRunClosed, pub.events)
	}
	payload, _ := event.Payload.(mwanachamataskmanager.WorkflowRunClosedPayload)
	if payload.WorkflowRunID != run.ID || payload.CancelRunID != cancelRun.ID || payload.StepID != "WI-7" {
		t.Errorf("payload = %+v", payload)
	}
}

func TestCloseWorkflowRunsPastLimit_ClosesARunOnlyOnce(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	at := minutesAfter(run, 6)
	if closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, at); err != nil || len(closed) != 1 {
		t.Fatalf("first sweep = %v, %v", closed, err)
	}
	if closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, at); err != nil || len(closed) != 0 {
		t.Fatalf("second sweep = %v, %v; want none", closed, err)
	}
	under, _ := mgr.ListWorkflowRunsUnderWay(ctx, "ride-ask")
	if len(under) != 0 {
		t.Errorf("runs under way for ride-ask = %d, want 0", len(under))
	}
}

func TestCloseWorkflowRunsPastLimit_SkipsARunThatWasAnswered(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	if _, err := mgr.AnswerWorkflowRunStep(ctx, run.ID); err != nil {
		t.Fatalf("AnswerWorkflowRunStep: %v", err)
	}
	if closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(run, 60)); err != nil || len(closed) != 0 {
		t.Fatalf("sweep = %v, %v; want none", closed, err)
	}
	got, _ := mgr.GetWorkflowRun(ctx, run.ID)
	if got.StepDueAt != "" || got.StepCancelWorkflowCode != "" || got.Status != mwanachamataskmanager.WorkflowRunStatusInProgress {
		t.Errorf("answered run = %+v", got)
	}
}

func TestCloseWorkflowRunsPastLimit_SkipsARunThatMovedToAStepWithNoLimit(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	if _, err := mgr.BeginWorkflowRunStep(ctx, run.ID, "WI-8", 0, ""); err != nil {
		t.Fatalf("BeginWorkflowRunStep: %v", err)
	}
	if closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(run, 60)); err != nil || len(closed) != 0 {
		t.Fatalf("sweep = %v, %v; want none", closed, err)
	}
}

func TestCloseWorkflowRunsPastLimit_SkipsARunThatEndedBeforeTheLimit(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusCompleted, ""); err != nil {
		t.Fatalf("completed: %v", err)
	}
	if closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(run, 60)); err != nil || len(closed) != 0 {
		t.Fatalf("sweep = %v, %v; want none", closed, err)
	}
	got, _ := mgr.GetWorkflowRun(ctx, run.ID)
	if got.Status != mwanachamataskmanager.WorkflowRunStatusCompleted {
		t.Errorf("status = %s, want completed", got.Status)
	}
}

func TestCloseWorkflowRunsPastLimit_ClosesAPausedRunToo(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusPaused, ""); err != nil {
		t.Fatalf("paused: %v", err)
	}
	closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(run, 6))
	if err != nil || len(closed) != 1 || closed[0].Status != mwanachamataskmanager.WorkflowRunStatusClosed {
		t.Fatalf("sweep = %v, %v; want the paused run closed", closed, err)
	}
}

func TestCloseWorkflowRunsPastLimit_FindsWaitingRunsBehindManyFinishedOnes(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	for i := 0; i < 600; i++ {
		finished, err := mgr.StartWorkflowRun(ctx, "", "ride-ask", 1, "", "")
		if err != nil {
			t.Fatalf("StartWorkflowRun: %v", err)
		}
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, finished.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
			t.Fatalf("in_progress: %v", err)
		}
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, finished.ID, mwanachamataskmanager.WorkflowRunStatusCompleted, ""); err != nil {
			t.Fatalf("completed: %v", err)
		}
	}
	run := waitingRun(t, mgr, 5)
	closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(run, 6))
	if err != nil || len(closed) != 1 || closed[0].ID != run.ID {
		t.Fatalf("sweep = %v, %v; want the waiting run", closed, err)
	}
}

func TestCloseWorkflowRunsPastLimit_ReusesACancelRunAlreadyOpenedForIt(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	existing, err := mgr.StartWorkflowRun(ctx, run.Name+"-closed", "ride-ask-cancel", 3, "step_limit_passed:"+run.ID, "watchdog")
	if err != nil {
		t.Fatalf("StartWorkflowRun: %v", err)
	}
	closed, err := mgr.CloseWorkflowRunsPastLimit(ctx, minutesAfter(run, 6))
	if err != nil || len(closed) != 1 || closed[0].CancelRunID != existing.ID {
		t.Fatalf("sweep = %v, %v; want the existing cancel run %s", closed, err, existing.ID)
	}
}

func TestCloseWorkflowRunsPastLimit_RefusesAnUnreadableInstant(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.CloseWorkflowRunsPastLimit(context.Background(), "tomorrow"); !errors.Is(err, mwanachamataskmanager.ErrInvalidTask) {
		t.Errorf("err = %v, want ErrInvalidTask", err)
	}
}

func TestUpdateWorkflowRunStatus_RefusesToSetClosed(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	run := waitingRun(t, mgr, 5)
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusClosed, ""); !errors.Is(err, mwanachamataskmanager.ErrInvalidRunStatusTransition) {
		t.Errorf("err = %v, want ErrInvalidRunStatusTransition", err)
	}
}
