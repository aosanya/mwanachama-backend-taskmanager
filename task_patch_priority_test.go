package mwanachamataskmanager_test

import (
	"context"
	"reflect"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func priorityOf(p mwanachamataskmanager.TaskPriority) *mwanachamataskmanager.TaskPriority { return &p }

func toDoTitles(t *testing.T, mgr mwanachamataskmanager.TaskManager) []string {
	t.Helper()
	tasks, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{Backlog: "backlog", Step: "to-do"})
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, 0, len(tasks))
	for _, task := range tasks {
		titles = append(titles, task.Title)
	}
	return titles
}

func threeInToDo(t *testing.T, mgr mwanachamataskmanager.TaskManager) []mwanachamataskmanager.Task {
	t.Helper()
	var made []mwanachamataskmanager.Task
	for _, title := range []string{"first", "second", "third"} {
		made = append(made, mustCreate(t, mgr, mwanachamataskmanager.Task{Title: title, Priority: mwanachamataskmanager.TaskPriorityMedium}))
	}
	return made
}

func TestPatchTask_RaisingToCriticalMovesTheIssueToTheTopOfItsStep(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := threeInToDo(t, mgr)

	got, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made[2].ID, Priority: priorityOf(mwanachamataskmanager.TaskPriorityCritical)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Priority != mwanachamataskmanager.TaskPriorityCritical {
		t.Fatalf("priority %q", got.Priority)
	}
	if want := []string{"third", "first", "second"}; !reflect.DeepEqual(toDoTitles(t, mgr), want) {
		t.Fatalf("order %v, want %v", toDoTitles(t, mgr), want)
	}
}

func TestPatchTask_ARaisedIssueSitsAtTheTopOfItsNewPriorityGroup(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := threeInToDo(t, mgr)
	critical := priorityOf(mwanachamataskmanager.TaskPriorityCritical)

	for _, id := range []string{made[1].ID, made[2].ID} {
		if _, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: id, Priority: critical}); err != nil {
			t.Fatal(err)
		}
	}
	if want := []string{"third", "second", "first"}; !reflect.DeepEqual(toDoTitles(t, mgr), want) {
		t.Fatalf("order %v, want %v", toDoTitles(t, mgr), want)
	}
}

func TestPatchTask_LoweringToLowMovesTheIssueBelowTheOthers(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := threeInToDo(t, mgr)

	if _, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made[0].ID, Priority: priorityOf(mwanachamataskmanager.TaskPriorityLow)}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"second", "third", "first"}; !reflect.DeepEqual(toDoTitles(t, mgr), want) {
		t.Fatalf("order %v, want %v", toDoTitles(t, mgr), want)
	}
}

func TestPatchTask_APatchThatLeavesThePriorityAloneLeavesTheOrderAlone(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := threeInToDo(t, mgr)

	if _, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made[2].ID, Priority: priorityOf(mwanachamataskmanager.TaskPriorityMedium), Title: text("third renamed")}); err != nil {
		t.Fatal(err)
	}
	if want := []string{"first", "second", "third renamed"}; !reflect.DeepEqual(toDoTitles(t, mgr), want) {
		t.Fatalf("order %v, want %v", toDoTitles(t, mgr), want)
	}
	notes, err := mgr.ListTaskNotes(ctx, "", made[2].ID)
	if err != nil || len(notes) != 0 {
		t.Fatalf("got %d notes, err %v, want none", len(notes), err)
	}
}

func TestPatchTask_ChangingThePriorityWritesOneLogEntry(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := threeInToDo(t, mgr)

	if _, err := mgr.PatchTaskAs(ctx, "owner-1", mwanachamataskmanager.TaskPatch{ID: made[1].ID, Priority: priorityOf(mwanachamataskmanager.TaskPriorityHigh)}); err != nil {
		t.Fatal(err)
	}
	notes, err := mgr.ListTaskNotes(ctx, "owner-1", made[1].ID)
	if err != nil || len(notes) != 1 {
		t.Fatalf("got %d notes, err %v, want one", len(notes), err)
	}
	if notes[0].Text != "Priority medium to high" || notes[0].AuthorID != "owner-1" || notes[0].Kind != "worker" {
		t.Fatalf("got %+v", notes[0])
	}
}
