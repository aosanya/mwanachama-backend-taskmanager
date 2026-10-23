package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func holdOutside(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task, holder string) mwanachamataskmanager.Task {
	t.Helper()
	task.Status = mwanachamataskmanager.TaskStatusBlocked
	task.HeldBy = holder
	out, err := mgr.UpdateTask(context.Background(), task)
	if err != nil {
		t.Fatalf("pending→blocked held by %q: %v", holder, err)
	}
	return out
}

func TestHeldBy_ABlockedTaskNamesItsOutsideHolder(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "fix the light"})

	out := holdOutside(t, mgr, task, "contractor:acme")
	if out.HeldBy != "contractor:acme" {
		t.Fatalf("UpdateTask HeldBy = %q, want contractor:acme", out.HeldBy)
	}
	got, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.HeldBy != "contractor:acme" || got.Status != mwanachamataskmanager.TaskStatusBlocked {
		t.Fatalf("GetTask = %q/%s, want contractor:acme/blocked", got.HeldBy, got.Status)
	}
}

func TestHeldBy_LeavingBlockedClearsTheHolder(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	held := holdOutside(t, mgr, task, "outside")

	held.Status = mwanachamataskmanager.TaskStatusPending
	out, err := mgr.UpdateTask(ctx, held)
	if err != nil {
		t.Fatalf("blocked→pending: %v", err)
	}
	if out.HeldBy != "" {
		t.Fatalf("HeldBy after leaving blocked = %q, want empty", out.HeldBy)
	}
	got, _ := mgr.GetTask(ctx, task.ID)
	if got.HeldBy != "" {
		t.Fatalf("stored HeldBy after leaving blocked = %q, want empty", got.HeldBy)
	}
}

func TestHeldBy_IsRefusedOutsideBlocked(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	task.HeldBy = "outside"
	out, err := mgr.UpdateTask(ctx, task)
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if out.HeldBy != "" {
		t.Fatalf("pending task kept HeldBy %q, want empty", out.HeldBy)
	}
}

func TestHeldBy_ListTellsOutsideWorkApart(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	outside, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "outside"})
	inside, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "inside"})
	other, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "other holder"})
	holdOutside(t, mgr, outside, "provider-1")
	holdOutside(t, mgr, inside, "")
	holdOutside(t, mgr, other, "provider-2")

	got, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{HeldBy: "provider-1"})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 1 || got[0].ID != outside.ID {
		t.Fatalf("ListTasks held_by=provider-1 = %v, want only %s", got, outside.ID)
	}

	all, _ := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Status: mwanachamataskmanager.TaskStatusBlocked})
	holders := map[string]string{}
	for _, task := range all {
		holders[task.ID] = task.HeldBy
	}
	if holders[outside.ID] != "provider-1" || holders[inside.ID] != "" || holders[other.ID] != "provider-2" {
		t.Fatalf("blocked listing holders = %v", holders)
	}
}
