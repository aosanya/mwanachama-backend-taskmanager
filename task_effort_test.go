package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestCreateTask_EffortSentIsKept(t *testing.T) {
	mgr := newTestManager(t)
	made := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "sized", Effort: mwanachamataskmanager.TaskEffortVeryHigh})
	read, err := mgr.GetTask(context.Background(), made.ID)
	if err != nil {
		t.Fatal(err)
	}
	if made.Effort != mwanachamataskmanager.TaskEffortVeryHigh || read.Effort != mwanachamataskmanager.TaskEffortVeryHigh {
		t.Fatalf("effort created %q read %q", made.Effort, read.Effort)
	}
}

func TestCreateTask_NoEffortLandsAsUnknown(t *testing.T) {
	mgr := newTestManager(t)
	made := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "unsized"})
	read, err := mgr.GetTask(context.Background(), made.ID)
	if err != nil {
		t.Fatal(err)
	}
	if made.Effort != mwanachamataskmanager.TaskEffortUnknown || read.Effort != mwanachamataskmanager.TaskEffortUnknown {
		t.Fatalf("effort created %q read %q", made.Effort, read.Effort)
	}
}

func TestCreateTask_EffortOutsideTheListIsRefused(t *testing.T) {
	mgr := newTestManager(t)
	_, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "bad", Effort: "huge"})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidEffort) {
		t.Fatalf("err %v, want ErrInvalidEffort", err)
	}
}

func TestPatchTask_EffortChangesAndNothingElseDoes(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := newTaggedIssue(t, mgr)
	effort := mwanachamataskmanager.TaskEffortLow
	patched, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made.ID, Effort: &effort})
	if err != nil {
		t.Fatal(err)
	}
	if patched.Effort != mwanachamataskmanager.TaskEffortLow || patched.Title != "HM1" || patched.Priority != mwanachamataskmanager.TaskPriorityCritical || len(patched.Tags) != 2 {
		t.Fatalf("patched %+v", patched)
	}
}

func TestPatchTask_EffortOutsideTheListIsRefusedAndNothingChanges(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "kept", Effort: mwanachamataskmanager.TaskEffortMedium})
	bad := mwanachamataskmanager.TaskEffort("enormous")
	_, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made.ID, Effort: &bad})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidEffort) {
		t.Fatalf("err %v, want ErrInvalidEffort", err)
	}
	read, err := mgr.GetTask(ctx, made.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Effort != mwanachamataskmanager.TaskEffortMedium {
		t.Fatalf("effort %q, want medium", read.Effort)
	}
}

func TestUpdateTask_AnEmptyEffortKeepsTheStoredOne(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "kept", Effort: mwanachamataskmanager.TaskEffortHigh})
	made.Effort = ""
	if _, err := mgr.UpdateTask(ctx, made); err != nil {
		t.Fatal(err)
	}
	read, err := mgr.GetTask(ctx, made.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.Effort != mwanachamataskmanager.TaskEffortHigh {
		t.Fatalf("effort %q, want high", read.Effort)
	}
}
