package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func titlesOf(tasks []mwanachamataskmanager.Task) map[string]bool {
	out := map[string]bool{}
	for _, task := range tasks {
		out[task.Title] = true
	}
	return out
}

func mustCreate(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) mwanachamataskmanager.Task {
	t.Helper()
	out, err := mgr.CreateTask(context.Background(), task)
	if err != nil {
		t.Fatalf("CreateTask %q: %v", task.Title, err)
	}
	return out
}

func listTitles(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.TaskFilter) map[string]bool {
	t.Helper()
	tasks, err := mgr.ListTasks(context.Background(), filter)
	if err != nil {
		t.Fatalf("ListTasks %+v: %v", filter, err)
	}
	return titlesOf(tasks)
}

func wantTitles(t *testing.T, got map[string]bool, want ...string) {
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
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "docs one", Tags: []string{"Docs"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "docs two", Tags: []string{"Docs", "P1"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "plain"})

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs"}}), "docs one", "docs two")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"P1"}}), "docs two")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Nothing"}}))
}

func TestListTasks_NarrowsByRepo(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in hub", ProjectName: "hub-steward"})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in swe", ProjectName: "hub-steward-swe"})

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Repo: "hub-steward"}), "in hub")
}

func TestListTasks_NarrowsByStep(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	todo := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "waiting to start"})
	doing := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "being done"})
	doing.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, doing); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	_ = todo

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Step: "To do"}), "waiting to start")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Step: "doing"}), "being done")

	_, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Step: "Imaginary"})
	if !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklogStep) {
		t.Fatalf("unknown step error = %v, want ErrUnknownBacklogStep", err)
	}
}

func TestListTasks_BacklogIsTheFirstOneUntilMoreExist(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "only"})

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog"}), "only")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "BACKLOG"}), "only")
	_, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{Backlog: "Other"})
	if !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("unknown backlog error = %v, want ErrUnknownBacklog", err)
	}
}

func TestListTasks_NarrowsByDaysUntouched(t *testing.T) {
	mgr, db, tasks := newTestManagerWithDB(t)
	old := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "old"})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "fresh"})
	longAgo := time.Now().UTC().AddDate(0, 0, -40).Format(time.RFC3339)
	if err := db.Table(tasks).Where("id = ?", old.ID).UpdateColumn("updated_at", longAgo).Error; err != nil {
		t.Fatalf("age the task: %v", err)
	}

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{UntouchedDays: 30}), "old")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{UntouchedDays: 60}))
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{}), "old", "fresh")
}

func TestListTasks_FiltersCombine(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "match", Tags: []string{"Docs"}, ProjectName: "a"})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "wrong repo", Tags: []string{"Docs"}, ProjectName: "b"})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "no tag", ProjectName: "a"})

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs"}, Repo: "a", Step: "To do"}), "match")
}

func TestListTasks_SeveralTagsMeanEveryTag(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "both", Tags: []string{"Docs", "Blocked"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "docs only", Tags: []string{"Docs"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "blocked only", Tags: []string{"Blocked"}})

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs", "Blocked"}}), "both")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs", "Missing"}}))
}

func TestListTasks_TagMatchingIgnoresCase(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "docs", Tags: []string{"Docs"}})

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"docs"}}), "docs")
}

func TestListTasks_NegativeUntouchedDaysIsRefused(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{UntouchedDays: -5})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidFilter) {
		t.Fatalf("negative untouched_days error = %v, want ErrInvalidFilter", err)
	}
}

func TestListTasks_BacklogAndStepFollowTheStoredPlace(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateBacklog(ctx, "Release", []string{"Plan", "Ship"}); err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	first := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "unplaced"})
	moved := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "moved"})
	if _, err := mgr.MoveTaskToStep(ctx, moved.ID, "release", "ship"); err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}
	_ = first

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "Release"}), "moved")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "Release", Step: "ship"}), "moved")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "Release", Step: "Plan"}))
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog"}), "unplaced")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "To do"}), "unplaced")
}

func TestTaskFilter_TagAcceptsOneNameOrAList(t *testing.T) {
	for raw, want := range map[string]int{`{"tag":"Docs"}`: 1, `{"tag":["Docs","Blocked"]}`: 2, `{}`: 0} {
		var filter mwanachamataskmanager.TaskFilter
		if err := json.Unmarshal([]byte(raw), &filter); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if len(filter.Tag) != want {
			t.Fatalf("%s gave %v, want %d tags", raw, filter.Tag, want)
		}
	}
	var bad mwanachamataskmanager.TaskFilter
	if err := json.Unmarshal([]byte(`{"tag":5}`), &bad); err == nil {
		t.Fatal("a number is not a tag")
	}
}

func TestListTasks_EmptyTagIsIgnored(t *testing.T) {
	mgr := newTestManager(t)
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "any"})

	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{""}}), "any")
}
