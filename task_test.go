package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// ── NewTaskManager ───────────────────────────────────────────────────────────

func TestNewTaskManager_NilDataManager(t *testing.T) {
	_, err := mwanachamataskmanager.NewTaskManager(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil data manager, got nil")
	}
}

func TestNewTaskManager_ValidDataManager(t *testing.T) {
	mgr, err := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil TaskManager")
	}
}

// ── CreateTask ───────────────────────────────────────────────────────────────

func TestCreateTask_Success(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	task, err := mgr.CreateTask(context.Background(), "agency-1", mwanachamataskmanager.Task{})
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
	if task.AgencyID != "agency-1" {
		t.Errorf("want agencyID agency-1, got %s", task.AgencyID)
	}
}

func TestCreateTask_PublishesEvent(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	if _, err := mgr.CreateTask(context.Background(), "agency-1", mwanachamataskmanager.Task{}); err != nil {
		t.Fatal(err)
	}
	// No "|agencyID" suffix here — unlike the original's eventbus.Event,
	// events.Publisher.Publish(ctx, topic, payload) carries no separate
	// agency envelope, so recordingPublisher.events is topic-only (see
	// fake_test.go).
	if len(pub.events) != 1 || pub.events[0] != "task.created" {
		t.Errorf("expected task.created event, got %v", pub.events)
	}
}

// ── GetTask ──────────────────────────────────────────────────────────────────

func TestGetTask_NotFound(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.GetTask(context.Background(), "agency-1", "nonexistent")
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestGetTask_Found(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "agency-1", mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(context.Background(), "agency-1", created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("want ID %q, got %q", created.ID, got.ID)
	}
}

// ── UpdateTask ───────────────────────────────────────────────────────────────

func TestUpdateTask_NotFound(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.UpdateTask(context.Background(), "agency-1", mwanachamataskmanager.Task{
		ID: "nonexistent", Status: mwanachamataskmanager.TaskStatusInProgress,
	})
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestUpdateTask_InvalidTransition_PendingToCompleted(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "a", mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusCompleted
	_, err = mgr.UpdateTask(context.Background(), "a", created)
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidStatusTransition) {
		t.Fatalf("want ErrInvalidStatusTransition, got %v", err)
	}
}

func TestUpdateTask_ValidTransition_PendingToInProgress(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "a", mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusInProgress
	updated, err := mgr.UpdateTask(context.Background(), "a", created)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != mwanachamataskmanager.TaskStatusInProgress {
		t.Errorf("want in_progress, got %s", updated.Status)
	}
}

func TestUpdateTask_ValidTransition_InProgressToCompleted(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), pub)
	created, err := mgr.CreateTask(context.Background(), "a", mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "a", created); err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusCompleted
	updated, err := mgr.UpdateTask(context.Background(), "a", created)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != mwanachamataskmanager.TaskStatusCompleted {
		t.Errorf("want completed, got %s", updated.Status)
	}
	// created, status.changed (pending→in_progress), status.changed
	// (in_progress→completed), completed. The terminal completed hook
	// fires AFTER the matching status.changed event so subscribers see
	// the transition before the terminal signal.
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
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "a", mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "a", created); err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusCompleted
	if _, err := mgr.UpdateTask(context.Background(), "a", created); err != nil {
		t.Fatal(err)
	}
	created.Status = mwanachamataskmanager.TaskStatusPending
	_, err = mgr.UpdateTask(context.Background(), "a", created)
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidStatusTransition) {
		t.Fatalf("want ErrInvalidStatusTransition, got %v", err)
	}
}

// ── DeleteTask ───────────────────────────────────────────────────────────────

func TestDeleteTask_NotFound(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	err := mgr.DeleteTask(context.Background(), "agency-1", "nonexistent")
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestDeleteTask_Success(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "a", mwanachamataskmanager.Task{})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteTask(context.Background(), "a", created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = mgr.GetTask(context.Background(), "a", created.ID)
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound after delete, got %v", err)
	}
}

// ── ListTasks ────────────────────────────────────────────────────────────────

func TestListTasks_EmptyAgency(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	tasks, err := mgr.ListTasks(context.Background(), "agency-1", mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("want 0 tasks, got %d", len(tasks))
	}
}

func TestListTasks_FilterByStatus(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	var ids []string
	for range 3 {
		created, err := mgr.CreateTask(context.Background(), "a", mwanachamataskmanager.Task{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.ID)
	}
	first, err := mgr.GetTask(context.Background(), "a", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	first.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "a", first); err != nil {
		t.Fatal(err)
	}

	pending, err := mgr.ListTasks(context.Background(), "a", mwanachamataskmanager.TaskFilter{
		Status: mwanachamataskmanager.TaskStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("want 2 pending tasks, got %d", len(pending))
	}

	inProgress, err := mgr.ListTasks(context.Background(), "a", mwanachamataskmanager.TaskFilter{
		Status: mwanachamataskmanager.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inProgress) != 1 {
		t.Errorf("want 1 in_progress task, got %d", len(inProgress))
	}
}

func TestListTasks_AgencyIsolation(t *testing.T) {
	mgr, _ := mwanachamataskmanager.NewTaskManager(newFakeDataManager(), nil)
	if _, err := mgr.CreateTask(context.Background(), "agency-A", mwanachamataskmanager.Task{}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateTask(context.Background(), "agency-B", mwanachamataskmanager.Task{}); err != nil {
		t.Fatal(err)
	}

	tasksA, err := mgr.ListTasks(context.Background(), "agency-A", mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasksA) != 1 {
		t.Errorf("agency-A: want 1 task, got %d", len(tasksA))
	}
	if tasksA[0].AgencyID != "agency-A" {
		t.Errorf("expected agency-A task, got agencyID=%s", tasksA[0].AgencyID)
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
