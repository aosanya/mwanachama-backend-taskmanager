package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestATaskCreatedWithALinkedRefReadsItBack(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	const ref = "forms/agy1f2e3d4c/form-123"

	created, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Answer the survey", LinkedRef: ref})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.LinkedRef != ref {
		t.Fatalf("created linked_ref = %q, want %q", created.LinkedRef, ref)
	}

	got, err := mgr.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.LinkedRef != ref {
		t.Fatalf("read linked_ref = %q, want %q", got.LinkedRef, ref)
	}

	listed, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(listed) != 1 || listed[0].LinkedRef != ref {
		t.Fatalf("listed = %+v, want one task carrying %q", listed, ref)
	}
}

func TestATaskWithoutALinkedRefIsCreatedAndReadsEmpty(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "No questionnaire"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	got, err := mgr.GetTask(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.LinkedRef != "" {
		t.Fatalf("linked_ref = %q, want empty", got.LinkedRef)
	}
}

func TestUpdatingATaskCanSetAndClearItsLinkedRef(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()

	created, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Later linked"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	created.LinkedRef = "forms/agy1f2e3d4c/form-9"
	if _, err := mgr.UpdateTask(ctx, created); err != nil {
		t.Fatalf("UpdateTask set: %v", err)
	}
	got, _ := mgr.GetTask(ctx, created.ID)
	if got.LinkedRef != "forms/agy1f2e3d4c/form-9" {
		t.Fatalf("after set linked_ref = %q", got.LinkedRef)
	}

	got.LinkedRef = ""
	if _, err := mgr.UpdateTask(ctx, got); err != nil {
		t.Fatalf("UpdateTask clear: %v", err)
	}
	cleared, _ := mgr.GetTask(ctx, created.ID)
	if cleared.LinkedRef != "" {
		t.Fatalf("after clear linked_ref = %q, want empty", cleared.LinkedRef)
	}
}
