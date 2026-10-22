package mwanachamataskmanager_test

import (
	"context"
	"testing"
	"time"

	tm "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestCancellingARunClosesTheCaseOfItsLastOpenTask(t *testing.T) {
	ctx := context.Background()
	m, pub := newManagerWithPublisher(t)
	run := createRunAtStatus(t, m, tm.WorkflowRunStatusInProgress)
	c, _ := m.OpenCase(ctx, "resident-1", tm.Case{})

	outside, err := m.CreateTask(ctx, tm.Task{Title: "outside the run"})
	if err != nil {
		t.Fatal(err)
	}
	inside, err := m.CreateTask(ctx, tm.Task{Title: "inside the run", WorkflowRunID: run.ID})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{outside.ID, inside.ID} {
		if err := m.LinkCaseTask(ctx, c.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	setStatus(t, m, outside.ID, tm.TaskStatusCancelled)
	if got, _ := m.GetCase(ctx, c.ID); got.Status != tm.CaseStatusOpen {
		t.Fatalf("case closed with a task still open: %s", got.Status)
	}

	if _, err := m.CancelWorkflowRun(ctx, run.ID, "stop", "alice", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, _ := m.GetCase(ctx, c.ID)
	if got.Status != tm.CaseStatusDone || got.ClosedAt == "" {
		t.Fatalf("case = %+v, want done after the cascade cancelled its last task", got)
	}
	if countTopic(pub, tm.TopicCaseClosed) != 1 {
		t.Fatalf("case.closed published %d times", countTopic(pub, tm.TopicCaseClosed))
	}
}

func TestCancellingARunLeavesACaseWithAnotherOpenTaskOpen(t *testing.T) {
	ctx := context.Background()
	m, pub := newManagerWithPublisher(t)
	run := createRunAtStatus(t, m, tm.WorkflowRunStatusInProgress)
	c, _ := m.OpenCase(ctx, "resident-1", tm.Case{})

	outside, _ := m.CreateTask(ctx, tm.Task{Title: "outside the run"})
	inside, _ := m.CreateTask(ctx, tm.Task{Title: "inside the run", WorkflowRunID: run.ID})
	for _, id := range []string{outside.ID, inside.ID} {
		if err := m.LinkCaseTask(ctx, c.ID, id); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := m.CancelWorkflowRun(ctx, run.ID, "stop", "alice", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.GetCase(ctx, c.ID); got.Status != tm.CaseStatusOpen {
		t.Fatalf("case = %s, want open while a task outside the run is open", got.Status)
	}
	if countTopic(pub, tm.TopicCaseClosed) != 0 {
		t.Fatal("case.closed published for a case with an open task")
	}
}

func TestTimingOutATaskFailsItAndKeepsItsCaseOpen(t *testing.T) {
	ctx := context.Background()
	m, pub := newManagerWithPublisher(t)
	c, _ := m.OpenCase(ctx, "resident-1", tm.Case{})
	task, _ := m.CreateTask(ctx, tm.Task{Title: "slow"})
	if err := m.LinkCaseTask(ctx, c.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	setStatus(t, m, task.ID, tm.TaskStatusInProgress)

	if err := m.HandleTaskTimeout(ctx, task.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.GetTask(ctx, task.ID); got.Status != tm.TaskStatusFailed {
		t.Fatalf("task = %s, want failed", got.Status)
	}
	if got, _ := m.GetCase(ctx, c.ID); got.Status != tm.CaseStatusOpen {
		t.Fatalf("case = %s, want open: a failed task does not settle a case", got.Status)
	}
	if countTopic(pub, tm.TopicCaseClosed) != 0 {
		t.Fatal("case.closed published for a failed task")
	}
}

func TestRollingBackARunLeavesItsTasksCaseOpenAndDoesNotReopenADoneOne(t *testing.T) {
	ctx := context.Background()
	m, pub := newManagerWithPublisher(t)

	openRun, _ := m.CreateWorkflowRun(ctx, "", "", "")
	open, _ := m.OpenCase(ctx, "resident-1", tm.Case{})
	pending, _ := m.CreateTask(ctx, tm.Task{Title: "reset", WorkflowRunID: openRun.ID})
	if err := m.LinkTaskToRun(ctx, openRun.ID, pending.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkCaseTask(ctx, open.ID, pending.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := rollBack(t, m, openRun.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.GetCase(ctx, open.ID); got.Status != tm.CaseStatusOpen {
		t.Fatalf("case = %s, want open after its task was reset to pending", got.Status)
	}

	doneRun, _ := m.CreateWorkflowRun(ctx, "", "", "")
	done, _ := m.OpenCase(ctx, "resident-2", tm.Case{})
	finished, _ := m.CreateTask(ctx, tm.Task{Title: "finished", WorkflowRunID: doneRun.ID})
	if err := m.LinkTaskToRun(ctx, doneRun.ID, finished.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkCaseTask(ctx, done.ID, finished.ID); err != nil {
		t.Fatal(err)
	}
	setStatus(t, m, finished.ID, tm.TaskStatusCancelled)
	if got, _ := m.GetCase(ctx, done.ID); got.Status != tm.CaseStatusDone {
		t.Fatalf("case = %s, want done before the rollback", got.Status)
	}
	if _, err := rollBack(t, m, doneRun.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.GetCase(ctx, done.ID); got.Status != tm.CaseStatusDone {
		t.Fatalf("case = %s, want still done: a rollback does not reopen a case", got.Status)
	}
	if countTopic(pub, tm.TopicCaseClosed) != 1 {
		t.Fatalf("case.closed published %d times, want once", countTopic(pub, tm.TopicCaseClosed))
	}
}
