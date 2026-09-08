package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// ── NewTaskManager ───────────────────────────────────────────────────────────
// TestNewTaskManager_NilDB lives in testdb_test.go, alongside newTestManager.

func TestNewTaskManager_ValidDB(t *testing.T) {
	mgr := newTestManager(t)
	if mgr == nil {
		t.Fatal("expected non-nil TaskManager")
	}
}

// ── CreateTask ───────────────────────────────────────────────────────────────

func TestCreateTask_Success(t *testing.T) {
	mgr := newTestManager(t)
	task, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID == "" {
		t.Errorf("expected server-generated ID, got empty")
	}
	if task.Status != mwanachamataskmanager.TaskStatusPending {
		t.Errorf("want status pending, got %s", task.Status)
	}
	if task.Priority != mwanachamataskmanager.TaskPriorityMedium {
		t.Errorf("want default priority medium, got %s", task.Priority)
	}
}

func TestCreateTask_PublishesEvent(t *testing.T) {
	pub := &recordingPublisher{}
	mgr := newTestManagerWithPublisher(t, pub)
	if _, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{}); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 || pub.events[0] != "task.created" {
		t.Errorf("expected task.created event, got %v", pub.events)
	}
}

// ── GetTask ──────────────────────────────────────────────────────────────────

func TestGetTask_NotFound(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.GetTask(context.Background(), "nonexistent")
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestGetTask_Found(t *testing.T) {
	mgr := newTestManager(t)
	created, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("want ID %q, got %q", created.ID, got.ID)
	}
}

// ── UpdateTask ───────────────────────────────────────────────────────────────

func TestUpdateTask_NotFound(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.UpdateTask(context.Background(), mwanachamataskmanager.Task{
		ID: "nonexistent", Status: mwanachamataskmanager.TaskStatusInProgress,
	})
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestUpdateTask_InvalidTransition_PendingToCompleted(t *testing.T) {
	mgr := newTestManager(t)
	created, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusCompleted
	_, err = mgr.UpdateTask(context.Background(), created)
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidStatusTransition) {
		t.Fatalf("want ErrInvalidStatusTransition, got %v", err)
	}
}

func TestUpdateTask_ValidTransition_PendingToInProgress(t *testing.T) {
	mgr := newTestManager(t)
	created, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusInProgress
	updated, err := mgr.UpdateTask(context.Background(), created)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != mwanachamataskmanager.TaskStatusInProgress {
		t.Errorf("want in_progress, got %s", updated.Status)
	}
}

func TestUpdateTask_ValidTransition_InProgressToCompleted(t *testing.T) {
	pub := &recordingPublisher{}
	mgr := newTestManagerWithPublisher(t, pub)
	created, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusCompleted
	updated, err := mgr.UpdateTask(context.Background(), created)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != mwanachamataskmanager.TaskStatusCompleted {
		t.Errorf("want completed, got %s", updated.Status)
	}
	want := []string{
		"task.created",
		"task.status.changed",
		"task.status.changed",
		"task.completed",
	}
	if len(pub.events) != len(want) {
		t.Fatalf("event count: got %d (%v), want %d (%v)", len(pub.events), pub.events, len(want), want)
	}
	for i := range want {
		if pub.events[i] != want[i] {
			t.Errorf("event[%d]: got %q, want %q", i, pub.events[i], want[i])
		}
	}
}

func TestUpdateTask_InvalidTransition_CompletedToPending(t *testing.T) {
	mgr := newTestManager(t)
	created, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusCompleted
	if _, err := mgr.UpdateTask(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusPending
	_, err = mgr.UpdateTask(context.Background(), created)
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidStatusTransition) {
		t.Fatalf("want ErrInvalidStatusTransition, got %v", err)
	}
}

// ── DeleteTask ───────────────────────────────────────────────────────────────

func TestDeleteTask_NotFound(t *testing.T) {
	mgr := newTestManager(t)
	err := mgr.DeleteTask(context.Background(), "nonexistent")
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestDeleteTask_Success(t *testing.T) {
	mgr := newTestManager(t)
	created, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteTask(context.Background(), created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = mgr.GetTask(context.Background(), created.ID)
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound after delete, got %v", err)
	}
}

// ── ListTasks ────────────────────────────────────────────────────────────────

func TestListTasks_Empty(t *testing.T) {
	mgr := newTestManager(t)
	tasks, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("want 0 tasks, got %d", len(tasks))
	}
}

func TestListTasks_FilterByStatus(t *testing.T) {
	mgr := newTestManager(t)
	var ids []string
	for range 3 {
		created, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.ID)
	}
	first, err := mgr.GetTask(context.Background(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	first.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), first); err != nil {
		t.Fatal(err)
	}

	pending, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{
		Status: mwanachamataskmanager.TaskStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("want 2 pending tasks, got %d", len(pending))
	}

	inProgress, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{
		Status: mwanachamataskmanager.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inProgress) != 1 {
		t.Errorf("want 1 in_progress task, got %d", len(inProgress))
	}
}

func TestListTasks_ReturnsAllTasks(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{}); err != nil {
		t.Fatal(err)
	}

	tasks, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Errorf("want 2 tasks, got %d", len(tasks))
	}
}

// ── TaskStatus.CanTransitionTo ───────────────────────────────────────────────

func TestCanTransitionTo(t *testing.T) {
	tests := []struct {
		from  mwanachamataskmanager.TaskStatus
		to    mwanachamataskmanager.TaskStatus
		allow bool
	}{
		{mwanachamataskmanager.TaskStatusPending, mwanachamataskmanager.TaskStatusInProgress, true},
		{mwanachamataskmanager.TaskStatusPending, mwanachamataskmanager.TaskStatusCancelled, true},
		{mwanachamataskmanager.TaskStatusPending, mwanachamataskmanager.TaskStatusCompleted, false},
		{mwanachamataskmanager.TaskStatusPending, mwanachamataskmanager.TaskStatusFailed, false},
		{mwanachamataskmanager.TaskStatusInProgress, mwanachamataskmanager.TaskStatusCompleted, true},
		{mwanachamataskmanager.TaskStatusInProgress, mwanachamataskmanager.TaskStatusFailed, true},
		{mwanachamataskmanager.TaskStatusInProgress, mwanachamataskmanager.TaskStatusCancelled, true},
		{mwanachamataskmanager.TaskStatusInProgress, mwanachamataskmanager.TaskStatusPending, false},
		{mwanachamataskmanager.TaskStatusCompleted, mwanachamataskmanager.TaskStatusPending, false},
		{mwanachamataskmanager.TaskStatusCompleted, mwanachamataskmanager.TaskStatusInProgress, false},
		{mwanachamataskmanager.TaskStatusFailed, mwanachamataskmanager.TaskStatusPending, false},
		{mwanachamataskmanager.TaskStatusCancelled, mwanachamataskmanager.TaskStatusPending, false},
	}
	for _, tc := range tests {
		got := tc.from.CanTransitionTo(tc.to)
		if got != tc.allow {
			t.Errorf("CanTransitionTo(%s → %s): want %v, got %v", tc.from, tc.to, tc.allow, got)
		}
	}
}
