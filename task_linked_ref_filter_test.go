package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func seedLinkedJobs(t *testing.T, mgr mwanachamataskmanager.TaskManager) {
	t.Helper()
	ctx := context.Background()
	for _, task := range []mwanachamataskmanager.Task{
		{Title: "mend the pump", LinkedRef: "asset-a"},
		{Title: "paint the pump house", LinkedRef: "asset-a"},
		{Title: "clear the culvert", LinkedRef: "asset-b"},
		{Title: "unlinked chore"},
	} {
		made := mustCreate(t, mgr, task)
		if task.Title == "mend the pump" || task.Title == "clear the culvert" {
			made.Status = mwanachamataskmanager.TaskStatusInProgress
			if _, err := mgr.UpdateTask(ctx, made); err != nil {
				t.Fatal(err)
			}
			made.Status = mwanachamataskmanager.TaskStatusCompleted
			if _, err := mgr.UpdateTask(ctx, made); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestListTasks_NarrowsByLinkedRef(t *testing.T) {
	mgr := newTestManager(t)
	seedLinkedJobs(t, mgr)

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{LinkedRef: "asset-a"}), "mend the pump", "paint the pump house")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{LinkedRef: "asset-b"}), "clear the culvert")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{LinkedRef: "asset-missing"}))
}

func TestListTasks_LinkedRefComposesWithStatusAndLimit(t *testing.T) {
	mgr := newTestManager(t)
	seedLinkedJobs(t, mgr)

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{LinkedRef: "asset-a", Status: mwanachamataskmanager.TaskStatusCompleted}), "mend the pump")
	tasks, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{LinkedRef: "asset-a", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("limit 1 returned %d tasks", len(tasks))
	}
}

func TestPendingTasks_NarrowsByLinkedRef(t *testing.T) {
	mgr := newTestManager(t)
	seedLinkedJobs(t, mgr)

	out, err := mgr.PendingTasks(context.Background(), mwanachamataskmanager.PendingFilter{LinkedRef: "asset-a"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 1 || out.Items[0].Title != "paint the pump house" {
		t.Fatalf("pending for asset-a: %+v", out.Items)
	}
}
