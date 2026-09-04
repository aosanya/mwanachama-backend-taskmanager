package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// setBlocks wires source --[blocks]--> target.
func setBlocks(t *testing.T, mgr mwanachamataskmanager.TaskManager, sourceID, targetID string) {
	t.Helper()
	if _, err := mgr.CreateRelationship(context.Background(), mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelBlocks, FromID: sourceID, ToID: targetID,
	}); err != nil {
		t.Fatalf("CreateRelationship blocks %s→%s: %v", sourceID, targetID, err)
	}
}

// startTask drives a freshly-created task pending → in_progress, returning
// any error from UpdateTask. Used to exercise the blocker gate.
func startTask(mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) error {
	task.Status = mwanachamataskmanager.TaskStatusInProgress
	_, err := mgr.UpdateTask(context.Background(), task)
	return err
}

// completeBlocker drives a task pending → in_progress → completed. Used to
// stand up scenarios where a blocker has reached terminal state.
func completeBlocker(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) {
	t.Helper()
	task.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("blocker pending→in_progress: %v", err)
	}
	task.Status = mwanachamataskmanager.TaskStatusCompleted
	if _, err := mgr.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("blocker in_progress→completed: %v", err)
	}
}

// cancelTask drives a task pending → cancelled.
func cancelTask(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) {
	t.Helper()
	task.Status = mwanachamataskmanager.TaskStatusCancelled
	if _, err := mgr.UpdateTask(context.Background(), task); err != nil {
		t.Fatalf("pending→cancelled: %v", err)
	}
}

func TestUpdateTask_Blocked_PendingBlockerPreventsStart(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	b, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	setBlocks(t, mgr, a.ID, b.ID)

	err := startTask(mgr, b)
	if !errors.Is(err, mwanachamataskmanager.ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
	var be *mwanachamataskmanager.BlockedError
	if !errors.As(err, &be) {
		t.Fatalf("want *BlockedError via errors.As, got %T", err)
	}
	if len(be.BlockerTaskIDs) != 1 || be.BlockerTaskIDs[0] != a.ID {
		t.Errorf("BlockerTaskIDs = %v, want [%s]", be.BlockerTaskIDs, a.ID)
	}
}

func TestUpdateTask_Blocked_CompletedBlockerOpensGate(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	b, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	setBlocks(t, mgr, a.ID, b.ID)

	completeBlocker(t, mgr, a)
	if err := startTask(mgr, b); err != nil {
		t.Errorf("after blocker completed: got %v, want nil", err)
	}
}

func TestUpdateTask_Blocked_CancelledBlockerOpensGate(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	b, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	setBlocks(t, mgr, a.ID, b.ID)

	cancelTask(t, mgr, a)
	if err := startTask(mgr, b); err != nil {
		t.Errorf("after blocker cancelled: got %v, want nil", err)
	}
}

func TestUpdateTask_Blocked_MultipleBlockers_OnlyNonTerminalReported(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a1, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	a2, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	b, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	setBlocks(t, mgr, a1.ID, b.ID)
	setBlocks(t, mgr, a2.ID, b.ID)

	completeBlocker(t, mgr, a1) // a1 terminal, a2 still pending

	err := startTask(mgr, b)
	var be *mwanachamataskmanager.BlockedError
	if !errors.As(err, &be) {
		t.Fatalf("want *BlockedError, got %v", err)
	}
	if len(be.BlockerTaskIDs) != 1 {
		t.Fatalf("want 1 blocker, got %d (%v)", len(be.BlockerTaskIDs), be.BlockerTaskIDs)
	}
	if be.BlockerTaskIDs[0] != a2.ID {
		t.Errorf("blocker = %s, want %s (the still-pending one)", be.BlockerTaskIDs[0], a2.ID)
	}
}

func TestUpdateTask_Blocked_PendingToCancelledBypassesGate(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	b, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	setBlocks(t, mgr, a.ID, b.ID)

	// pending → cancelled must succeed even when the blocker is still pending.
	b.Status = mwanachamataskmanager.TaskStatusCancelled
	if _, err := mgr.UpdateTask(ctx, b); err != nil {
		t.Errorf("pending→cancelled with active blocker: got %v, want nil", err)
	}
}

func TestUpdateTask_Blocked_DependsOnIsNotGated(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	b, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})

	// depends_on is informational — it must not block the transition.
	if _, err := mgr.CreateRelationship(ctx, mwanachamataskmanager.Relationship{
		Label: mwanachamataskmanager.RelLabelDependsOn, FromID: b.ID, ToID: a.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship depends_on: %v", err)
	}
	_ = a

	if err := startTask(mgr, b); err != nil {
		t.Errorf("depends_on with non-terminal target: got %v, want nil", err)
	}
}

func TestBlockedError_IsAndAs(t *testing.T) {
	be := &mwanachamataskmanager.BlockedError{BlockerTaskIDs: []string{"x", "y"}}
	if !errors.Is(be, mwanachamataskmanager.ErrBlocked) {
		t.Error("errors.Is(BlockedError, ErrBlocked) = false, want true")
	}
	var got *mwanachamataskmanager.BlockedError
	if !errors.As(be, &got) {
		t.Error("errors.As did not extract *BlockedError")
	}
	want := []string{"x", "y"}
	gotIDs := append([]string(nil), got.BlockerTaskIDs...)
	sort.Strings(gotIDs)
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("ids: got %v, want %v", gotIDs, want)
			break
		}
	}
}
