package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func completeAt(t *testing.T, mgr mwanachamataskmanager.TaskManager, stamp func(id, at string), title, at string) mwanachamataskmanager.Task {
	t.Helper()
	created := createOrderTask(t, mgr, mwanachamataskmanager.Task{Title: title})
	if _, err := mgr.MoveTaskToStep(context.Background(), created.ID, "backlog", "doing"); err != nil {
		t.Fatalf("move %q to doing: %v", title, err)
	}
	done, err := mgr.MoveTaskToStep(context.Background(), created.ID, "backlog", "done")
	if err != nil {
		t.Fatalf("move %q to done: %v", title, err)
	}
	if done.CompletedAt == "" {
		t.Fatalf("%q reached Done with no completed_at", title)
	}
	stamp(done.ID, at)
	return done
}

func orderedTitles(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.TaskFilter) []string {
	t.Helper()
	tasks, err := mgr.ListTasks(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListTasks %+v: %v", filter, err)
	}
	titles := make([]string, len(tasks))
	for i, task := range tasks {
		titles[i] = task.Title
	}
	return titles
}

func wantOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestListTasks_TheDoneStepListsTheLatestCompletionFirst(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	stamp := func(id, at string) {
		if err := db.Table(table).Where("id = ?", id).UpdateColumn("completed_at", at).Error; err != nil {
			t.Fatal(err)
		}
	}
	completeAt(t, mgr, stamp, "first created, finished second", "2026-11-11T10:00:00Z")
	completeAt(t, mgr, stamp, "second created, finished last", "2026-11-11T12:00:00Z")
	completeAt(t, mgr, stamp, "third created, finished first", "2026-11-11T09:00:00Z")

	wantOrder(t, orderedTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "done"}),
		"second created, finished last", "first created, finished second", "third created, finished first")
}

func TestListTasks_AnIssueThatReturnsToDoneIsPlacedByItsLatestCompletion(t *testing.T) {
	ctx := context.Background()
	mgr, db, table := newTestManagerWithDB(t)
	stamp := func(id, at string) {
		if err := db.Table(table).Where("id = ?", id).UpdateColumn("completed_at", at).Error; err != nil {
			t.Fatal(err)
		}
	}
	completeAt(t, mgr, stamp, "finished once", "2026-11-11T12:00:00Z")
	back := completeAt(t, mgr, stamp, "finished again", "2026-11-11T09:00:00Z")
	if err := db.Table(table).Where("id = ?", back.ID).UpdateColumn("status", "in_progress").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, back.ID, "backlog", "done"); err != nil {
		t.Fatalf("move back to done: %v", err)
	}

	wantOrder(t, orderedTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "done"}),
		"finished again", "finished once")
}

func TestListTasks_EveryOtherStepKeepsRankOrder(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	low := createOrderTask(t, mgr, mwanachamataskmanager.Task{Title: "low rank", Rank: 10})
	high := createOrderTask(t, mgr, mwanachamataskmanager.Task{Title: "high rank", Rank: 20})
	for _, id := range []string{high.ID, low.ID} {
		if _, err := mgr.MoveTaskToStep(ctx, id, "backlog", "doing"); err != nil {
			t.Fatalf("move to doing: %v", err)
		}
	}

	wantOrder(t, orderedTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "doing"}), "low rank", "high rank")
}

func createOrderTask(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) mwanachamataskmanager.Task {
	t.Helper()
	out, err := mgr.CreateTask(context.Background(), task)
	if err != nil {
		t.Fatalf("CreateTask %q: %v", task.Title, err)
	}
	return out
}
