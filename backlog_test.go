package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func stepIDs(b mwanachamataskmanager.BacklogSummary) []string {
	ids := make([]string, len(b.Steps))
	for i, s := range b.Steps {
		ids[i] = s.ID
	}
	return ids
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBacklogManager_StartWithOneBacklogOfFiveSteps(t *testing.T) {
	mgr := newTestManager(t)
	list, err := mgr.ListBacklogs(context.Background())
	if err != nil {
		t.Fatalf("ListBacklogs: %v", err)
	}
	if len(list.Items) != 1 || !equalIDs(stepIDs(list.Items[0]), []string{"to-do", "doing", "waiting", "done", "cancelled"}) {
		t.Fatalf("got %+v", list)
	}
}

func TestBacklogManager_NewTaskReadsAtTheStepOfItsStatus(t *testing.T) {
	mgr := newTestManager(t)
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	got, err := mgr.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Backlog != "backlog" || got.Step != "to-do" {
		t.Fatalf("got %q at %q", got.Backlog, got.Step)
	}
	list, _ := mgr.ListBacklogs(context.Background())
	if list.Items[0].Issues != 1 || list.Items[0].Steps[0].Issues != 1 {
		t.Fatalf("got %+v", list.Items[0])
	}
}

func TestBacklogManager_MoveToStepWalksTheStatusAndPlacesTheTask(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	moved, err := mgr.MoveTaskToStep(ctx, task.ID, "backlog", "doing")
	if err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}
	if moved.Status != mwanachamataskmanager.TaskStatusInProgress || moved.Step != "doing" {
		t.Fatalf("got %s at %q", moved.Status, moved.Step)
	}
}

func TestBacklogManager_AnIssueMovesToAnotherBacklogAndUpdatesKeepTheStep(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	created, err := mgr.CreateBacklog(ctx, "Research", []string{"Ideas", "Reading", "Writing"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	if created.ID != "research" || !equalIDs(stepIDs(created), []string{"ideas", "reading", "writing"}) {
		t.Fatalf("got %+v", created)
	}
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	moved, err := mgr.MoveTaskToStep(ctx, task.ID, "research", "reading")
	if err != nil {
		t.Fatal(err)
	}
	if moved.Status != mwanachamataskmanager.TaskStatusPending {
		t.Fatalf("a step that names no status left the status alone, got %s", moved.Status)
	}
	moved.Title = "renamed"
	updated, err := mgr.UpdateTask(ctx, moved)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := mgr.GetTask(ctx, updated.ID)
	if again.Backlog != "research" || again.Step != "reading" || again.Title != "renamed" {
		t.Fatalf("got %q at %q titled %q", again.Backlog, again.Step, again.Title)
	}
}

func TestBacklogManager_AStepHoldingIssuesCannotBeRemoved(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateBacklog(ctx, "Research", []string{"Ideas", "Reading"}); err != nil {
		t.Fatal(err)
	}
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "research", "reading"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.RemoveBacklogStep(ctx, "research", "reading"); !errors.Is(err, mwanachamataskmanager.ErrBacklogHoldsIssues) {
		t.Fatalf("got %v", err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "research", "ideas"); err != nil {
		t.Fatal(err)
	}
	left, err := mgr.RemoveBacklogStep(ctx, "research", "reading")
	if err != nil || !equalIDs(stepIDs(left), []string{"ideas"}) {
		t.Fatalf("got %+v, %v", left, err)
	}
}

func TestBacklogManager_ABacklogHoldingIssuesCannotBeDeleted(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateBacklog(ctx, "Research", nil); err != nil {
		t.Fatal(err)
	}
	task := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "research", "to-do"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteBacklog(ctx, "research"); !errors.Is(err, mwanachamataskmanager.ErrBacklogHoldsIssues) {
		t.Fatalf("got %v", err)
	}
	createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "unplaced"})
	if err := mgr.DeleteBacklog(ctx, "backlog"); !errors.Is(err, mwanachamataskmanager.ErrBacklogHoldsIssues) {
		t.Fatalf("the first backlog holds the unplaced issues, got %v", err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "backlog", "to-do"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteBacklog(ctx, "research"); err != nil {
		t.Fatalf("got %v", err)
	}
}

func TestBacklogManager_StepsAreAddedRenamedAndReordered(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateBacklog(ctx, "Research", []string{"Ideas", "Writing"}); err != nil {
		t.Fatal(err)
	}
	added, err := mgr.AddBacklogStep(ctx, "research", "Reading", "ideas")
	if err != nil || !equalIDs(stepIDs(added), []string{"ideas", "reading", "writing"}) {
		t.Fatalf("got %+v, %v", added, err)
	}
	renamed, err := mgr.RenameBacklogStep(ctx, "research", "reading", "Review")
	if err != nil || renamed.Steps[1].Name != "Review" || renamed.Steps[1].ID != "reading" {
		t.Fatalf("got %+v, %v", renamed, err)
	}
	ordered, err := mgr.ReorderBacklogSteps(ctx, "research", []string{"writing", "ideas", "reading"})
	if err != nil || !equalIDs(stepIDs(ordered), []string{"writing", "ideas", "reading"}) {
		t.Fatalf("got %+v, %v", ordered, err)
	}
	if _, err := mgr.ReorderBacklogSteps(ctx, "research", []string{"writing", "ideas"}); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("got %v", err)
	}
	if _, err := mgr.RenameBacklog(ctx, "nope", "x"); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("got %v", err)
	}
	if _, err := mgr.AddBacklogStep(ctx, "research", "Review", ""); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("a repeated name is refused, got %v", err)
	}
}

func TestBacklogManager_ReplaceKeepsAtLeastOneBacklog(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.ReplaceBacklogs(context.Background(), nil); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("got %v", err)
	}
}

func createForBacklogTest(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) mwanachamataskmanager.Task {
	t.Helper()
	out, err := mgr.CreateTask(context.Background(), task)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return out
}
