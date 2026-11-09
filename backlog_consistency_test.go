package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func placeOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) (string, string) {
	t.Helper()
	got, err := mgr.GetTask(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return got.Backlog, got.Step
}

func twoStepBacklog() []models.Backlog {
	return []models.Backlog{{ID: "work", Name: "Work", Steps: []models.BacklogStep{
		{ID: "open", Name: "Open", Statuses: []models.TaskStatus{models.TaskStatusPending}},
		{ID: "closed", Name: "Closed", Statuses: []models.TaskStatus{models.TaskStatusCompleted}},
	}}}
}

func TestBacklogConsistency_AStatusChangeMovesAPlacedTaskToTheStepThatNamesTheStatus(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "backlog", "doing"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToState(ctx, task.ID, "Done"); err != nil {
		t.Fatal(err)
	}
	if backlog, step := placeOf(t, mgr, task.ID); backlog != "backlog" || step != "done" {
		t.Fatalf("got %q at %q, want backlog at done", backlog, step)
	}
	summary := summaryOf(t, mgr, "backlog")
	if doing, _ := stepIssues(summary, "doing"); doing != 0 {
		t.Fatalf("Doing still counts the task: %+v", summary)
	}
	if done, _ := stepIssues(summary, "done"); done != 1 {
		t.Fatalf("Done does not count the task: %+v", summary)
	}
}

func TestBacklogConsistency_AStatusNoStepNamesLeavesTheTaskAtNoStepButInItsBacklog(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	if _, err := mgr.ReplaceBacklogs(ctx, twoStepBacklog()); err != nil {
		t.Fatal(err)
	}
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "work", "open"); err != nil {
		t.Fatal(err)
	}
	moved := task
	moved.Status = mwanachamataskmanager.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, moved); err != nil {
		t.Fatal(err)
	}
	if backlog, step := placeOf(t, mgr, task.ID); backlog != "work" || step != "" {
		t.Fatalf("got %q at %q, want work at no step", backlog, step)
	}
	summary := summaryOf(t, mgr, "work")
	if open, _ := stepIssues(summary, "open"); open != 0 || summary.Issues != 1 {
		t.Fatalf("the Open column must not claim a task whose status it does not name: %+v", summary)
	}
}

func TestBacklogConsistency_AStepThatNamesNoStatusKeepsTheTaskWhenItsStatusChanges(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	if _, err := mgr.CreateBacklog(ctx, "Research", []string{"Ideas", "Reading"}); err != nil {
		t.Fatal(err)
	}
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "research", "reading"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToState(ctx, task.ID, "Doing"); err != nil {
		t.Fatal(err)
	}
	if backlog, step := placeOf(t, mgr, task.ID); backlog != "research" || step != "reading" {
		t.Fatalf("got %q at %q, want research at reading", backlog, step)
	}
}

func TestBacklogConsistency_ReorderingTheFirstBacklogKeepsUnplacedIssuesWhereTheyWere(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	unplaced := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "loaded from a board"})
	research := models.Backlog{ID: "research", Name: "Research", Steps: []models.BacklogStep{{ID: "ideas", Name: "Ideas"}}}
	next := append([]models.Backlog{research}, models.DefaultBacklogs()...)
	if _, err := mgr.ReplaceBacklogs(ctx, next); err != nil {
		t.Fatal(err)
	}
	if backlog, step := placeOf(t, mgr, unplaced.ID); backlog != "backlog" || step != "to-do" {
		t.Fatalf("got %q at %q, want backlog at to-do", backlog, step)
	}
	if issues := summaryOf(t, mgr, "research").Issues; issues != 0 {
		t.Fatalf("the new first backlog took %d issues", issues)
	}
}

func TestBacklogConsistency_ClearingAStepsStatusesKeepsUnplacedIssuesInThatStep(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	unplaced := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	next := models.DefaultBacklogs()
	next[0].Steps[0].Statuses = nil
	if _, err := mgr.ReplaceBacklogs(ctx, next); err != nil {
		t.Fatal(err)
	}
	if backlog, step := placeOf(t, mgr, unplaced.ID); backlog != "backlog" || step != "to-do" {
		t.Fatalf("got %q at %q, want backlog at to-do", backlog, step)
	}
	if todo, _ := stepIssues(summaryOf(t, mgr, "backlog"), "to-do"); todo != 1 {
		t.Fatalf("To do lost its issue")
	}
}

func TestBacklogConsistency_ATaskNamingAnUnknownBacklogReadsAsUnplacedAndDoesNotBlockSaves(t *testing.T) {
	ctx := context.Background()
	mgr, db, tables := newTestManagerWithDB(t, nil)
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if err := db.Table(tables.Tasks).Where("id = ?", task.ID).Updates(map[string]any{"backlog": "ghost", "step": "x"}).Error; err != nil {
		t.Fatal(err)
	}
	if backlog, step := placeOf(t, mgr, task.ID); backlog != "backlog" || step != "to-do" {
		t.Fatalf("got %q at %q, want backlog at to-do", backlog, step)
	}
	if _, err := mgr.RenameBacklog(ctx, "backlog", "Renamed"); err != nil {
		t.Fatalf("a task of an unknown backlog blocked a save: %v", err)
	}
}

func TestBacklogConsistency_AFailedMoveLeavesThePlaceAndTheStatusAsTheyWere(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.MoveTaskToState(ctx, task.ID, "Done"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "backlog", "to-do"); !errors.Is(err, mwanachamataskmanager.ErrNoPathToState) {
		t.Fatalf("got %v", err)
	}
	after, err := mgr.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != mwanachamataskmanager.TaskStatusCompleted || after.Step != "done" {
		t.Fatalf("got %s at %q", after.Status, after.Step)
	}
}
