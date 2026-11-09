package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func summaryOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, backlogID string) mwanachamataskmanager.BacklogSummary {
	t.Helper()
	list, err := mgr.ListBacklogs(context.Background())
	if err != nil {
		t.Fatalf("ListBacklogs: %v", err)
	}
	for _, item := range list.Items {
		if item.ID == backlogID {
			return item
		}
	}
	t.Fatalf("backlog %q not listed in %+v", backlogID, list.Items)
	return mwanachamataskmanager.BacklogSummary{}
}

func stepIssues(summary mwanachamataskmanager.BacklogSummary, stepID string) (int, bool) {
	for _, step := range summary.Steps {
		if step.ID == stepID {
			return step.Issues, true
		}
	}
	return 0, false
}

func TestBacklogs_StartWithOneBacklogOfTheFiveStates(t *testing.T) {
	mgr := newTestManager(t)
	list, err := mgr.ListBacklogs(context.Background())
	if err != nil {
		t.Fatalf("ListBacklogs: %v", err)
	}
	if len(list.Items) != 1 || len(list.Items[0].Steps) != 5 {
		t.Fatalf("a new hub lists %+v, want one backlog of five steps", list.Items)
	}
}

func TestBacklogs_AStepHoldingIssuesCannotBeRemoved(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made, err := mgr.CreateBacklog(ctx, "Software", []string{"Ideas", "Build", "Review"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	issue := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "write the loader"})
	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, made.ID, "build"); err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}

	_, err = mgr.RemoveBacklogStep(ctx, made.ID, "build")
	if !errors.Is(err, mwanachamataskmanager.ErrBacklogHoldsIssues) {
		t.Fatalf("removing a step that holds an issue = %v, want ErrBacklogHoldsIssues", err)
	}
	if n, ok := stepIssues(summaryOf(t, mgr, made.ID), "build"); !ok || n != 1 {
		t.Fatalf("after the refused removal the step holds %d issues (present %v), want 1", n, ok)
	}

	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, made.ID, "review"); err != nil {
		t.Fatalf("MoveTaskToStep out: %v", err)
	}
	if _, err := mgr.RemoveBacklogStep(ctx, made.ID, "build"); err != nil {
		t.Fatalf("removing the emptied step = %v, want success", err)
	}
	if _, ok := stepIssues(summaryOf(t, mgr, made.ID), "build"); ok {
		t.Fatal("the emptied step is still listed after removal")
	}
}

func TestBacklogs_ABacklogHoldingIssuesCannotBeDeleted(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made, err := mgr.CreateBacklog(ctx, "Software", []string{"Ideas", "Build"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	issue := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "write the loader"})
	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, made.ID, "ideas"); err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}

	if err := mgr.DeleteBacklog(ctx, made.ID); !errors.Is(err, mwanachamataskmanager.ErrBacklogHoldsIssues) {
		t.Fatalf("deleting a backlog that holds an issue = %v, want ErrBacklogHoldsIssues", err)
	}
	summaryOf(t, mgr, made.ID)

	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, "backlog", "to-do"); err != nil {
		t.Fatalf("moving the issue back to the first backlog: %v", err)
	}
	if err := mgr.DeleteBacklog(ctx, made.ID); err != nil {
		t.Fatalf("deleting the emptied backlog = %v, want success", err)
	}
}

func TestBacklogs_ReplaceCannotSidestepTheRefusals(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made, err := mgr.CreateBacklog(ctx, "Software", []string{"Ideas", "Build"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	issue := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "write the loader"})
	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, made.ID, "build"); err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}
	_, err = mgr.ReplaceBacklogs(ctx, []mwanachamataskmanager.Backlog{
		{ID: made.ID, Name: "Software", Steps: []mwanachamataskmanager.BacklogStep{{ID: "ideas", Name: "Ideas"}}},
	})
	if !errors.Is(err, mwanachamataskmanager.ErrBacklogHoldsIssues) {
		t.Fatalf("replacing the lists without a step that holds an issue = %v, want ErrBacklogHoldsIssues", err)
	}
	_, err = mgr.ReplaceBacklogs(ctx, nil)
	if err == nil {
		t.Fatal("replacing every backlog with none succeeded while an issue sits in one")
	}
}

func TestBacklogs_AnIssueIsInOneBacklogAtOneStep(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	a, err := mgr.CreateBacklog(ctx, "Software", []string{"Ideas", "Build"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	b, err := mgr.CreateBacklog(ctx, "Docs", []string{"Draft", "Final"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	issue := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "one place only"})

	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, a.ID, "build"); err != nil {
		t.Fatalf("move into %s: %v", a.ID, err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, b.ID, "final"); err != nil {
		t.Fatalf("move into %s: %v", b.ID, err)
	}

	if n, _ := stepIssues(summaryOf(t, mgr, a.ID), "build"); n != 0 {
		t.Fatalf("%s/build still holds %d issues after the move away", a.ID, n)
	}
	if n, _ := stepIssues(summaryOf(t, mgr, b.ID), "final"); n != 1 {
		t.Fatalf("%s/final holds %d issues, want 1", b.ID, n)
	}
	if got := summaryOf(t, mgr, a.ID).Issues + summaryOf(t, mgr, b.ID).Issues; got != 1 {
		t.Fatalf("the issue is counted %d times across the two backlogs, want once", got)
	}
}

func TestBacklogs_MovingToAStepThatDoesNotExistIsRefused(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made, err := mgr.CreateBacklog(ctx, "Software", []string{"Ideas", "Build"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	issue := createForBacklogTest(t, mgr, mwanachamataskmanager.Task{Title: "stay put"})
	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, made.ID, "build"); err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}

	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, made.ID, "no-such-step"); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklogStep) {
		t.Fatalf("move to an unknown step = %v, want ErrUnknownBacklogStep", err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, issue.ID, "no-such-backlog", "build"); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("move to an unknown backlog = %v, want ErrUnknownBacklog", err)
	}
	if n, _ := stepIssues(summaryOf(t, mgr, made.ID), "build"); n != 1 {
		t.Fatalf("after refused moves the issue left its step: build holds %d", n)
	}
}
