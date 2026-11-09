package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func createFilterTask(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) mwanachamataskmanager.Task {
	t.Helper()
	out, err := mgr.CreateTask(context.Background(), task)
	if err != nil {
		t.Fatalf("CreateTask %q: %v", task.Title, err)
	}
	return out
}

func filterListTitles(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.TaskFilter) map[string]bool {
	t.Helper()
	tasks, err := mgr.ListTasks(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListTasks %+v: %v", filter, err)
	}
	out := map[string]bool{}
	for _, task := range tasks {
		out[task.Title] = true
	}
	return out
}

func wantFilterTitles(t *testing.T, got map[string]bool, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for _, title := range want {
		if !got[title] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestListTasks_NarrowsByTag(t *testing.T) {
	mgr := newTestManager(t)
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "docs one", Tags: []string{"Docs"}})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "docs two", Tags: []string{"Docs", "P1"}})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "plain"})

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs"}}), "docs one", "docs two")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"P1"}}), "docs two")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Nothing"}}))
}

func TestListTasks_NarrowsByRepo(t *testing.T) {
	mgr := newTestManager(t)
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "in hub", ProjectName: "hub-steward"})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "in swe", ProjectName: "hub-steward-swe"})

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Repo: "hub-steward"}), "in hub")
}

func TestListTasks_NarrowsByStep(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "waiting to start"})
	doing := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "being done"})
	doing.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, doing); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Step: "To do"}), "waiting to start")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Step: "doing"}), "being done")

	_, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Step: "Imaginary"})
	if !errors.Is(err, mwanachamataskmanager.ErrUnknownTaskState) {
		t.Fatalf("unknown step error = %v, want ErrUnknownTaskState", err)
	}
}

func TestListTasks_OnlyTheDefaultBacklogExistsForNow(t *testing.T) {
	mgr := newTestManager(t)
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "only"})

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "default"}), "only")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "DEFAULT"}), "only")
	_, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{Backlog: "Other"})
	if !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("unknown backlog error = %v, want ErrUnknownBacklog", err)
	}
}

func TestListTasks_NarrowsByDaysUntouched(t *testing.T) {
	mgr, db, tables := newTestManagerWithDB(t, nil)
	old := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "old"})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "fresh"})
	longAgo := time.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339)
	if err := db.Table(tables.Tasks).Where("id = ?", old.ID).UpdateColumn("updated_at", longAgo).Error; err != nil {
		t.Fatalf("age the task: %v", err)
	}

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{UntouchedDays: 30}), "old")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{UntouchedDays: 60}))
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{}), "old", "fresh")
}

func TestListTasks_FiltersCombine(t *testing.T) {
	mgr := newTestManager(t)
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "match", Tags: []string{"Docs"}, ProjectName: "a"})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "wrong repo", Tags: []string{"Docs"}, ProjectName: "b"})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "no tag", ProjectName: "a"})

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs"}, Repo: "a", Step: "To do"}), "match")
}

func TestListTasks_SeveralTagsMeanEveryTag(t *testing.T) {
	mgr := newTestManager(t)
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "both", Tags: []string{"Docs", "Blocked"}})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "docs only", Tags: []string{"Docs"}})
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "blocked only", Tags: []string{"Blocked"}})

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs", "Blocked"}}), "both")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs", "Missing"}}))
}

func TestListTasks_TagMatchingIgnoresCase(t *testing.T) {
	mgr := newTestManager(t)
	createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "docs", Tags: []string{"Docs"}})

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"docs"}}), "docs")
}

func TestListTasks_NegativeUntouchedDaysIsRefused(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{UntouchedDays: -5})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
		t.Fatalf("negative untouched_days error = %v, want ErrInvalidFilter", err)
	}
}
