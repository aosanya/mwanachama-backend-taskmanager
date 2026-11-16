package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"strings"
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
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
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
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
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
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
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
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
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
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "research", "to-do"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteBacklog(ctx, "research"); !errors.Is(err, mwanachamataskmanager.ErrBacklogHoldsIssues) {
		t.Fatalf("got %v", err)
	}
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "unplaced"})
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

func TestMoveTaskToStep_AnEmptyStepIsRefusedAndTheIssueStaysPut(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	task := softwareTask(t, mgr)
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "software", ""); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklogStep) {
		t.Fatalf("an empty step: %v", err)
	}
	if got, err := mgr.GetTask(ctx, task.ID); err != nil || got.Step != task.Step {
		t.Fatalf("the issue moved: %+v %v", got, err)
	}
}

func TestBacklogManager_TheRefusalNamesHowManyIssuesAStepOrBacklogHolds(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateBacklog(ctx, "Research", []string{"Ideas", "Reading"}); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"a", "b", "c"} {
		task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: title})
		if _, err := mgr.MoveTaskToStep(ctx, task.ID, "research", "reading"); err != nil {
			t.Fatal(err)
		}
	}
	_, err := mgr.RemoveBacklogStep(ctx, "research", "reading")
	if err == nil || !strings.Contains(err.Error(), `step "Reading" of "Research" still holds 3 issues, move them first`) {
		t.Fatalf("removing a step with three issues: %v", err)
	}
	err = mgr.DeleteBacklog(ctx, "research")
	if err == nil || !strings.Contains(err.Error(), `"Research" still holds 3 issues, move them first`) {
		t.Fatalf("deleting a backlog with three issues: %v", err)
	}
	last, err := mgr.CreateBacklog(ctx, "Notes", nil)
	if err != nil {
		t.Fatal(err)
	}
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "d"})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, last.ID, "to-do"); err != nil {
		t.Fatal(err)
	}
	err = mgr.DeleteBacklog(ctx, last.ID)
	if err == nil || !strings.Contains(err.Error(), `"Notes" still holds 1 issue, move it first`) {
		t.Fatalf("deleting a backlog with one issue: %v", err)
	}
}
