package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestDefaultTaskStatesAreTheFiveNamedByDecision278(t *testing.T) {
	want := []string{"To do", "Doing", "Waiting", "Done", "Cancelled"}
	got := models.DefaultTaskStates()
	if len(got) != len(want) {
		t.Fatalf("got %d states, want %d", len(got), len(want))
	}
	for i, s := range got {
		if s.Name != want[i] {
			t.Errorf("state %d = %q, want %q", i, s.Name, want[i])
		}
	}
}

func TestEveryDefaultStateMapsOnlyOntoDeclaredStatuses(t *testing.T) {
	known := map[models.TaskStatus]bool{
		models.TaskStatusPending: true, models.TaskStatusInProgress: true,
		models.TaskStatusBlocked: true, models.TaskStatusAwaitingDirection: true,
		models.TaskStatusCompleted: true, models.TaskStatusFailed: true,
		models.TaskStatusCancelled: true, models.TaskStatusSplit: true,
	}
	seen := map[models.TaskStatus]string{}
	for _, s := range models.DefaultTaskStates() {
		for _, st := range s.Statuses {
			if !known[st] {
				t.Errorf("state %q maps onto unknown status %q", s.Name, st)
			}
			if prev, ok := seen[st]; ok {
				t.Errorf("status %q mapped by both %q and %q", st, prev, s.Name)
			}
			seen[st] = s.Name
		}
	}
}

func TestStateNameForAnUnmappedStatusIsEmpty(t *testing.T) {
	if got := models.StateNameFor(models.DefaultTaskStates(), models.TaskStatusFailed); got != "" {
		t.Errorf("failed maps to %q, want empty", got)
	}
}

func TestTaskReadsAnswerTheDefaultState(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	created, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "seat", Priority: models.TaskPriorityLow})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.State != "To do" {
		t.Errorf("create state = %q, want To do", created.State)
	}
	got, err := mgr.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.State != "To do" {
		t.Errorf("get state = %q, want To do", got.State)
	}
	got.Status = models.TaskStatusInProgress
	updated, err := mgr.UpdateTask(ctx, got)
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.State != "Doing" {
		t.Errorf("update state = %q, want Doing", updated.State)
	}
	list, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(list) != 1 || list[0].State != "Doing" {
		t.Errorf("list = %+v, want one task in Doing", list)
	}
}
