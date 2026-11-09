package mwanachamataskmanager_test

import (
	"context"
	"reflect"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func feedOneItemRequest() mwanachamataskmanager.OpenSourcedTasks {
	return mwanachamataskmanager.OpenSourcedTasks{
		Source: "feed",
		Items:  []mwanachamataskmanager.SourcedTaskItem{{Key: "k1", Title: "from a feed", DueAt: "2030-01-01T00:00:00Z"}},
	}
}

func generateOneSubjectRequest() mwanachamataskmanager.GenerateTasks {
	return mwanachamataskmanager.GenerateTasks{
		Run:      "r1",
		Template: models.TaskTemplate{Title: "per subject"},
		Subjects: []string{"s1"},
	}
}

func issueCount(t *testing.T, mgr mwanachamataskmanager.TaskManager) int {
	t.Helper()
	tasks, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	return len(tasks)
}

func TestSourcedAndGeneratedTasks_NeedCanMoveOnTheFirstBacklog(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)

	_, err := mgr.OpenSourcedTasksAs(ctx, "dev", feedOneItemRequest())
	wantForbidden(t, "sourced tasks with no access", err)
	_, err = mgr.GenerateTasksAs(ctx, "dev", generateOneSubjectRequest())
	wantForbidden(t, "generated tasks with no access", err)

	if _, err := mgr.GiveBacklogAccess(ctx, "backlog", devGroup, "see"); err != nil {
		t.Fatal(err)
	}
	_, err = mgr.OpenSourcedTasksAs(ctx, "dev", feedOneItemRequest())
	wantForbidden(t, "sourced tasks with Can see", err)
	_, err = mgr.GenerateTasksAs(ctx, "dev", generateOneSubjectRequest())
	wantForbidden(t, "generated tasks with Can see", err)
	if got := issueCount(t, mgr); got != 0 {
		t.Fatalf("refused calls created %d issues", got)
	}

	if _, err := mgr.GiveBacklogAccess(ctx, "backlog", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.OpenSourcedTasksAs(ctx, "dev", feedOneItemRequest()); err != nil {
		t.Fatalf("sourced tasks with Can move: %v", err)
	}
	if _, err := mgr.GenerateTasksAs(ctx, "dev", generateOneSubjectRequest()); err != nil {
		t.Fatalf("generated tasks with Can move: %v", err)
	}
	if got := issueCount(t, mgr); got != 2 {
		t.Fatalf("allowed calls created %d issues, want 2", got)
	}
}

func TestSourcedAndGeneratedTasks_ACallWithNoPersonAndTheOwnerGroupAreNotNarrowed(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	if _, err := mgr.OpenSourcedTasksAs(ctx, "", feedOneItemRequest()); err != nil {
		t.Fatalf("no person: %v", err)
	}
	if _, err := mgr.GenerateTasksAs(ctx, "owner", generateOneSubjectRequest()); err != nil {
		t.Fatalf("owner group: %v", err)
	}
}

func TestTodosDoNotCreateOrChangeAnIssue(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	parent := softwareTask(t, mgr)
	before, err := mgr.GetTask(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	count := issueCount(t, mgr)
	if _, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{Title: "step", Instructions: "do it", ParentTaskID: parent.ID}); err != nil {
		t.Fatalf("create todo: %v", err)
	}
	after, err := mgr.GetTask(ctx, parent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("a todo changed its parent issue: %v -> %v", before, after)
	}
	if got := issueCount(t, mgr); got != count {
		t.Fatalf("a todo changed the issue count from %d to %d", count, got)
	}
}
