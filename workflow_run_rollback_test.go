package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// helpers -------------------------------------------------------------------

func newManagerWithPublisher(t *testing.T) (mwanachamataskmanager.TaskManager, *recordingPublisher) {
	t.Helper()
	pub := &recordingPublisher{}
	return newTestManagerWithPublisher(t, pub), pub
}

// rollBack drives an existing run to a status a rollback can start from and
// then rolls it back. Compensation is step 3 of that sequence and is no
// longer reachable on its own, so a test asserting what compensation does
// asks for the whole rollback.
func rollBack(t *testing.T, mgr mwanachamataskmanager.TaskManager, runID, reason string) (mwanachamataskmanager.WorkflowRun, error) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []mwanachamataskmanager.WorkflowRunStatus{
		mwanachamataskmanager.WorkflowRunStatusInProgress,
		mwanachamataskmanager.WorkflowRunStatusFailed,
	} {
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, runID, s, ""); err != nil {
			t.Fatalf("UpdateWorkflowRunStatus → %s: %v", s, err)
		}
	}
	return mgr.RollbackWorkflowRun(ctx, runID, reason)
}

func createRunAtStatus(t *testing.T, mgr mwanachamataskmanager.TaskManager, target mwanachamataskmanager.WorkflowRunStatus) mwanachamataskmanager.WorkflowRun {
	t.Helper()
	ctx := context.Background()
	run, err := mgr.CreateWorkflowRun(ctx, "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	transitions := map[mwanachamataskmanager.WorkflowRunStatus][]mwanachamataskmanager.WorkflowRunStatus{
		mwanachamataskmanager.WorkflowRunStatusInProgress: {mwanachamataskmanager.WorkflowRunStatusInProgress},
		mwanachamataskmanager.WorkflowRunStatusCompleted:  {mwanachamataskmanager.WorkflowRunStatusInProgress, mwanachamataskmanager.WorkflowRunStatusCompleted},
		mwanachamataskmanager.WorkflowRunStatusFailed:     {mwanachamataskmanager.WorkflowRunStatusInProgress, mwanachamataskmanager.WorkflowRunStatusFailed},
	}
	for _, s := range transitions[target] {
		run, err = mgr.UpdateWorkflowRunStatus(ctx, run.ID, s, "")
		if err != nil {
			t.Fatalf("UpdateWorkflowRunStatus → %s: %v", s, err)
		}
	}
	return run
}

// ── RollbackWorkflowRun ────────────────────────────────────────────────────

func TestRollbackWorkflowRun_FailedRun_ReachesRolledBack(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, mwanachamataskmanager.WorkflowRunStatusFailed)

	result, err := mgr.RollbackWorkflowRun(ctx, run.ID, "test rollback")
	if err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}
	if result.Status != mwanachamataskmanager.WorkflowRunStatusRolledBack {
		t.Errorf("status = %s, want rolled_back", result.Status)
	}

	topics := pub.topicList()
	if !contains(topics, mwanachamataskmanager.TopicRunRollingBack) {
		t.Errorf("expected work.run.rolling_back event; got %v", topics)
	}
	if !contains(topics, mwanachamataskmanager.TopicRunRolledBack) {
		t.Errorf("expected work.run.rolled_back event; got %v", topics)
	}
}

func TestRollbackWorkflowRun_CompletedRun_ReachesRolledBack(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, mwanachamataskmanager.WorkflowRunStatusCompleted)

	result, err := mgr.RollbackWorkflowRun(ctx, run.ID, "undo completed run")
	if err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}
	if result.Status != mwanachamataskmanager.WorkflowRunStatusRolledBack {
		t.Errorf("status = %s, want rolled_back", result.Status)
	}
}

func TestRollbackWorkflowRun_PendingRun_ReturnsInvalidTransition(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run, err := mgr.CreateWorkflowRun(ctx, "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	_, err = mgr.RollbackWorkflowRun(ctx, run.ID, "")
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRunStatusTransition) {
		t.Errorf("err = %v, want ErrInvalidRunStatusTransition", err)
	}
}

// A run is only ever found in rolling_back if a previous rollback stopped
// part-way — nothing in the API can put it there, which is what W14 closed.
// The row is written directly here to stand in for that crash.
func TestRollbackWorkflowRun_AlreadyRollingBack_ReturnsConflict(t *testing.T) {
	ctx := context.Background()
	pub := &recordingPublisher{}
	mgr, db, tables := newTestManagerWithDB(t, pub)
	run := createRunAtStatus(t, mgr, mwanachamataskmanager.WorkflowRunStatusFailed)

	if err := db.Table(tables.WorkflowRuns).Where("id = ?", run.ID).
		UpdateColumn("status", string(mwanachamataskmanager.WorkflowRunStatusRollingBack)).Error; err != nil {
		t.Fatalf("simulate an unfinished rollback: %v", err)
	}

	_, err := mgr.RollbackWorkflowRun(ctx, run.ID, "")
	if !errors.Is(err, mwanachamataskmanager.ErrRollbackConflict) {
		t.Errorf("err = %v, want ErrRollbackConflict", err)
	}
}

func TestRollbackWorkflowRun_NotFound_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)

	_, err := mgr.RollbackWorkflowRun(ctx, "no-such-id", "")
	if !errors.Is(err, mwanachamataskmanager.ErrWorkflowRunNotFound) {
		t.Errorf("err = %v, want ErrWorkflowRunNotFound", err)
	}
}

// ── DeleteWorkflowRunArtifacts ─────────────────────────────────────────────

func TestDeleteWorkflowRunArtifacts_ResetsTasksToPendingAndEmitsEvents(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)

	run, err := mgr.CreateWorkflowRun(ctx, "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{
		Title:         "t1",
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := mgr.LinkTaskToRun(ctx, run.ID, task.ID); err != nil {
		t.Fatalf("LinkTaskToRun: %v", err)
	}

	if _, err := rollBack(t, mgr, run.ID, ""); err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}

	// Task must still exist, reset to pending with workflow_run_id cleared.
	after, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after rollback: %v", err)
	}
	if after.Status != mwanachamataskmanager.TaskStatusPending {
		t.Errorf("task status = %s, want pending", after.Status)
	}
	if after.WorkflowRunID != "" {
		t.Errorf("task.WorkflowRunID = %q, want empty", after.WorkflowRunID)
	}
	// work.task.rolled_back event must have fired.
	if !contains(pub.topicList(), mwanachamataskmanager.TopicTaskRolledBack) {
		t.Errorf("expected work.task.rolled_back event; got %v", pub.topicList())
	}
}

// Rollback must clear `completed_at` on every reset Task. If a "only write
// completed_at when non-empty" guard were silently keeping the stale
// timestamp in storage, the next legitimate completion event would surface a
// stale date.
func TestDeleteWorkflowRunArtifacts_ClearsStaleCompletedAt(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)

	run, err := mgr.CreateWorkflowRun(ctx, "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{
		Title:         "Task that completed once and is now being rolled back",
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := mgr.LinkTaskToRun(ctx, run.ID, task.ID); err != nil {
		t.Fatalf("LinkTaskToRun: %v", err)
	}

	// Drive the task to COMPLETED so completed_at gets populated by the
	// state-machine path (UpdateTask sets it on terminal transitions).
	task.Status = mwanachamataskmanager.TaskStatusInProgress
	if task, err = mgr.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask → in_progress: %v", err)
	}
	task.Status = mwanachamataskmanager.TaskStatusCompleted
	if task, err = mgr.UpdateTask(ctx, task); err != nil {
		t.Fatalf("UpdateTask → completed: %v", err)
	}
	if task.CompletedAt == "" {
		t.Fatalf("setup: expected completed_at to be populated after completion; got empty")
	}
	staleCompletedAt := task.CompletedAt

	// Now roll back. The task should reset to pending AND completed_at MUST be cleared.
	if _, err := rollBack(t, mgr, run.ID, ""); err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}

	after, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask after rollback: %v", err)
	}
	if after.Status != mwanachamataskmanager.TaskStatusPending {
		t.Errorf("post-rollback status = %s, want pending", after.Status)
	}
	if after.CompletedAt != "" {
		t.Errorf("rollback must clear completed_at; got %q (was %q before rollback)",
			after.CompletedAt, staleCompletedAt)
	}
	if after.WorkflowRunID != "" {
		t.Errorf("rollback must clear workflow_run_id; got %q", after.WorkflowRunID)
	}
}

// Todos created for a run must be deleted on rollback.
func TestDeleteWorkflowRunArtifacts_DeletesTaskTodosForRun(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)

	run, err := mgr.CreateWorkflowRun(ctx, "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "parent", WorkflowRunID: run.ID})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Seed: two todos for THIS run, one for a different run.
	mineA, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title: "mine-a", Instructions: "do a", ParentTaskID: task.ID,
		Ordinality: 1, WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo mine-a: %v", err)
	}
	mineB, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title: "mine-b", Instructions: "do b", ParentTaskID: task.ID,
		Ordinality: 2, WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo mine-b: %v", err)
	}
	other, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{
		Title: "other-run", Instructions: "do z", ParentTaskID: task.ID,
		Ordinality: 1, WorkflowRunID: "different-run-id",
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo other: %v", err)
	}

	if _, err := rollBack(t, mgr, run.ID, ""); err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}

	// mineA / mineB must be gone.
	if _, err := mgr.GetTaskTodo(ctx, mineA.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
		t.Errorf("expected mineA deleted (ErrTaskTodoNotFound); got err=%v", err)
	}
	if _, err := mgr.GetTaskTodo(ctx, mineB.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskTodoNotFound) {
		t.Errorf("expected mineB deleted (ErrTaskTodoNotFound); got err=%v", err)
	}
	// The other-run todo must still exist.
	if _, err := mgr.GetTaskTodo(ctx, other.ID); err != nil {
		t.Errorf("expected other-run todo to remain; got err=%v", err)
	}
}

func TestDeleteWorkflowRunArtifacts_NoArtifacts_IsNoOp(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run, err := mgr.CreateWorkflowRun(ctx, "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	if _, err := rollBack(t, mgr, run.ID, ""); err != nil {
		t.Errorf("RollbackWorkflowRun on an empty run: %v", err)
	}
}

func TestDeleteWorkflowRunArtifacts_ForeignRunDependency_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)

	runA, _ := mgr.CreateWorkflowRun(ctx, "run-a", "", "")
	runB, _ := mgr.CreateWorkflowRun(ctx, "run-b", "", "")

	taskA, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "a", WorkflowRunID: runA.ID})
	taskB, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "b", WorkflowRunID: runB.ID})

	// taskB (run B) depends on taskA (run A).
	// Deleting run A's artifacts would break run B's dependency.
	if _, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
		Label:  mwanachamataskmanager.RelLabelDependsOn,
		FromID: taskB.ID,
		ToID:   taskA.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	rolled, err := rollBack(t, mgr, runA.ID, "")
	if !errors.Is(err, mwanachamataskmanager.ErrForeignRunDependency) {
		t.Errorf("err = %v, want ErrForeignRunDependency", err)
	}
	// The refusal leaves the run saying so rather than claiming it rolled back.
	if rolled.Status != mwanachamataskmanager.WorkflowRunStatusRollbackFailed {
		t.Errorf("status = %s, want rollback_failed", rolled.Status)
	}
	// Run A's task keeps its anchor, because nothing was compensated.
	after, err := mgr.GetTask(ctx, taskA.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if after.WorkflowRunID != runA.ID {
		t.Errorf("task.WorkflowRunID = %q, want it still anchored to run A", after.WorkflowRunID)
	}
}

// W14: compensation is step 3 of a rollback, so it is refused on a run
// nobody is rolling back. Without this a live run's tasks could be reset and
// its todos soft-deleted while the run itself still reported in_progress.
func TestDeleteWorkflowRunArtifacts_RefusedWhenTheRunIsNotRollingBack(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)

	for _, status := range []mwanachamataskmanager.WorkflowRunStatus{
		mwanachamataskmanager.WorkflowRunStatusPending,
		mwanachamataskmanager.WorkflowRunStatusInProgress,
		mwanachamataskmanager.WorkflowRunStatusFailed,
		mwanachamataskmanager.WorkflowRunStatusCompleted,
	} {
		run := createRunAtStatus(t, mgr, status)
		task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "live", WorkflowRunID: run.ID})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}

		if err := mgr.DeleteWorkflowRunArtifacts(ctx, run.ID); !errors.Is(err, mwanachamataskmanager.ErrRollbackNotInProgress) {
			t.Errorf("%s: err = %v, want ErrRollbackNotInProgress", status, err)
		}
		after, err := mgr.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("%s: GetTask: %v", status, err)
		}
		if after.WorkflowRunID != run.ID {
			t.Errorf("%s: the refused call still reset the task's anchor to %q", status, after.WorkflowRunID)
		}
	}
}

// W14: the three states a rollback drives a run through are the
// coordinator's. A caller reaching rolled_back directly would be claiming a
// compensation that never ran.
func TestUpdateWorkflowRunStatus_RefusesTheRollbackStates(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)

	for _, status := range []mwanachamataskmanager.WorkflowRunStatus{
		mwanachamataskmanager.WorkflowRunStatusRollingBack,
		mwanachamataskmanager.WorkflowRunStatusRolledBack,
		mwanachamataskmanager.WorkflowRunStatusRollbackFailed,
	} {
		run := createRunAtStatus(t, mgr, mwanachamataskmanager.WorkflowRunStatusFailed)
		if _, err := mgr.UpdateWorkflowRunStatus(ctx, run.ID, status, ""); !errors.Is(err, mwanachamataskmanager.ErrInvalidRunStatusTransition) {
			t.Errorf("setting %s: err = %v, want ErrInvalidRunStatusTransition", status, err)
		}
		after, err := mgr.GetWorkflowRun(ctx, run.ID)
		if err != nil {
			t.Fatalf("GetWorkflowRun: %v", err)
		}
		if after.Status != mwanachamataskmanager.WorkflowRunStatusFailed {
			t.Errorf("setting %s moved the run to %s anyway", status, after.Status)
		}
	}
}

// The coordinator still reaches all three, which is what makes the refusal a
// gate rather than a wall.
func TestRollbackWorkflowRun_ReachesRolledBackThroughTheCoordinator(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, mwanachamataskmanager.WorkflowRunStatusFailed)

	rolled, err := mgr.RollbackWorkflowRun(ctx, run.ID, "done")
	if err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}
	if rolled.Status != mwanachamataskmanager.WorkflowRunStatusRolledBack {
		t.Errorf("status = %s, want rolled_back", rolled.Status)
	}
}

// ── RollbackWorkflowRun event ordering ────────────────────────────────────

func TestRollbackWorkflowRun_PublishesRollingBackBeforeRolledBack(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, mwanachamataskmanager.WorkflowRunStatusFailed)

	if _, err := mgr.RollbackWorkflowRun(ctx, run.ID, "ordered events test"); err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}

	topics := pub.topicList()
	rollingIdx, rolledIdx := -1, -1
	for i, tp := range topics {
		if tp == mwanachamataskmanager.TopicRunRollingBack {
			rollingIdx = i
		}
		if tp == mwanachamataskmanager.TopicRunRolledBack {
			rolledIdx = i
		}
	}
	if rollingIdx < 0 || rolledIdx < 0 {
		t.Fatalf("missing events: rolling_back=%d rolled_back=%d in %v", rollingIdx, rolledIdx, topics)
	}
	if rollingIdx >= rolledIdx {
		t.Errorf("rolling_back (idx %d) must precede rolled_back (idx %d)", rollingIdx, rolledIdx)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (p *recordingPublisher) topicList() []string {
	out := make([]string, len(p.full))
	for i, e := range p.full {
		out[i] = e.Topic
	}
	return out
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
