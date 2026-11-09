package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestListTasks_AStaleStoredBacklogIdIsInTheFirstBacklog(t *testing.T) {
	ctx := context.Background()
	mgr, db, tables := newTestManagerWithDB(t, nil)
	if _, err := mgr.CreateBacklog(ctx, "Release", []string{"Plan", "Ship"}); err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	stale := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "stale id"})
	empty := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "empty id"})
	moved := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "moved"})
	if err := db.Table(tables.Tasks).Where("id = ?", stale.ID).Updates(map[string]any{"backlog": "ghost", "step": "x"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table(tables.Tasks).Where("id = ?", empty.ID).Updates(map[string]any{"backlog": "", "step": ""}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, moved.ID, "release", "ship"); err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog"}), "stale id", "empty id")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "Release"}), "moved")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "Release", Step: "ship"}), "moved")
}

func TestListTasks_AStaleIdTaskMatchesTheStepItsStatusNamesAndNoOtherStep(t *testing.T) {
	ctx := context.Background()
	mgr, db, tables := newTestManagerWithDB(t, nil)
	stale := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "stale id"})
	if err := db.Table(tables.Tasks).Where("id = ?", stale.ID).Updates(map[string]any{"backlog": "ghost", "step": "x"}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Backlog != "backlog" || got.Step != "to-do" {
		t.Fatalf("GetTask places it at %q/%q, want backlog/to-do", got.Backlog, got.Step)
	}

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "To do"}), "stale id")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Step: "To do"}), "stale id")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "Doing"}))
}

func TestListTasks_AStaleIdTaskWhoseStatusNoStepNamesIsInTheBacklogAtNoStep(t *testing.T) {
	ctx := context.Background()
	mgr, db, tables := newTestManagerWithDB(t, nil)
	if _, err := mgr.ReplaceBacklogs(ctx, []models.Backlog{{ID: "backlog", Name: "Backlog", Steps: []models.BacklogStep{
		{ID: "open", Name: "Open", Statuses: []models.TaskStatus{models.TaskStatusPending}},
	}}}); err != nil {
		t.Fatalf("ReplaceBacklogs: %v", err)
	}
	stale := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "stale id"})
	if err := db.Table(tables.Tasks).Where("id = ?", stale.ID).Updates(map[string]any{"backlog": "ghost", "step": "x", "status": "completed"}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Backlog != "backlog" || got.Step != "" {
		t.Fatalf("GetTask places it at %q/%q, want backlog at no step", got.Backlog, got.Step)
	}

	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog"}), "stale id")
	wantFilterTitles(t, filterListTitles(t, mgr, mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "Open"}))
}
