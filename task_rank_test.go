package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func createTitled(t *testing.T, mgr mwanachamataskmanager.TaskManager, titles ...string) []mwanachamataskmanager.Task {
	t.Helper()
	var out []mwanachamataskmanager.Task
	for _, title := range titles {
		task, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: title})
		if err != nil {
			t.Fatalf("CreateTask %s: %v", title, err)
		}
		out = append(out, task)
	}
	return out
}

func listedTitles(t *testing.T, mgr mwanachamataskmanager.TaskManager) string {
	t.Helper()
	tasks, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	var titles []string
	for _, task := range tasks {
		titles = append(titles, task.Title)
	}
	return strings.Join(titles, ",")
}

func TestNewCardsLandAtTheBottomInTheOrderTheyWereCreated(t *testing.T) {
	mgr := newTestManager(t)
	made := createTitled(t, mgr, "a", "b", "c")
	if !(made[0].Rank < made[1].Rank && made[1].Rank < made[2].Rank) {
		t.Fatalf("ranks %d %d %d are not rising", made[0].Rank, made[1].Rank, made[2].Rank)
	}
	if got := listedTitles(t, mgr); got != "a,b,c" {
		t.Fatalf("list = %s", got)
	}
}

func TestACardPlacedBeforeTheFirstGoesToTheTop(t *testing.T) {
	mgr := newTestManager(t)
	made := createTitled(t, mgr, "a", "b", "c")
	placed, err := mgr.PlaceTask(context.Background(), made[2].ID, "", made[0].ID)
	if err != nil {
		t.Fatalf("PlaceTask: %v", err)
	}
	if placed.Rank >= made[0].Rank {
		t.Fatalf("rank %d is not above %d", placed.Rank, made[0].Rank)
	}
	if got := listedTitles(t, mgr); got != "c,a,b" {
		t.Fatalf("list = %s", got)
	}
}

func TestACardPlacedAfterTheLastGoesToTheBottom(t *testing.T) {
	mgr := newTestManager(t)
	made := createTitled(t, mgr, "a", "b", "c")
	if _, err := mgr.PlaceTask(context.Background(), made[0].ID, made[2].ID, ""); err != nil {
		t.Fatalf("PlaceTask: %v", err)
	}
	if got := listedTitles(t, mgr); got != "b,c,a" {
		t.Fatalf("list = %s", got)
	}
}

func TestACardPlacedBetweenTwoLeavesTheNeighboursRanksAlone(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	made := createTitled(t, mgr, "a", "b", "c")
	if _, err := mgr.PlaceTask(ctx, made[2].ID, made[0].ID, made[1].ID); err != nil {
		t.Fatalf("PlaceTask: %v", err)
	}
	if got := listedTitles(t, mgr); got != "a,c,b" {
		t.Fatalf("list = %s", got)
	}
	for _, before := range []mwanachamataskmanager.Task{made[0], made[1]} {
		after, _ := mgr.GetTask(ctx, before.ID)
		if after.Rank != before.Rank {
			t.Fatalf("%s was renumbered from %d to %d", before.Title, before.Rank, after.Rank)
		}
	}
}

func TestPlacingAfterOneCardPutsTheCardDirectlyBelowIt(t *testing.T) {
	mgr := newTestManager(t)
	made := createTitled(t, mgr, "a", "b", "c", "d")
	if _, err := mgr.PlaceTask(context.Background(), made[3].ID, made[0].ID, ""); err != nil {
		t.Fatalf("PlaceTask: %v", err)
	}
	if got := listedTitles(t, mgr); got != "a,d,b,c" {
		t.Fatalf("list = %s", got)
	}
}

func TestPlacingIntoTheSameGapAgainAndAgainKeepsTheOrderAfterTheRoomRunsOut(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	made := createTitled(t, mgr, "a", "b")
	want := []string{"a", "b"}
	for i := 0; i < 40; i++ {
		title := fmt.Sprintf("n%d", i)
		task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: title})
		if err != nil {
			t.Fatalf("CreateTask: %v", err)
		}
		if _, err := mgr.PlaceTask(ctx, task.ID, made[0].ID, made[1].ID); err != nil {
			t.Fatalf("PlaceTask %d: %v", i, err)
		}
		want = append([]string{want[0], title}, want[1:]...)
	}
	if got := listedTitles(t, mgr); got != strings.Join(want, ",") {
		t.Fatalf("list = %s\nwant %s", got, strings.Join(want, ","))
	}
}

func TestAnUpdateDoesNotMoveTheCard(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	made := createTitled(t, mgr, "a", "b")
	if _, err := mgr.PlaceTask(ctx, made[1].ID, "", made[0].ID); err != nil {
		t.Fatalf("PlaceTask: %v", err)
	}
	placed, _ := mgr.GetTask(ctx, made[1].ID)
	edit := placed
	edit.Rank = 0
	edit.Title = "b edited"
	updated, err := mgr.UpdateTask(ctx, edit)
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Rank != placed.Rank {
		t.Fatalf("rank %d became %d", placed.Rank, updated.Rank)
	}
	if got := listedTitles(t, mgr); got != "b edited,a" {
		t.Fatalf("list = %s", got)
	}
}

func TestAPlacementRefusesWhatItCannotHonour(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	made := createTitled(t, mgr, "a", "b", "c")
	if err := mgr.DeleteTask(ctx, made[2].ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	cases := []struct {
		name          string
		task          string
		after, before string
		want          error
	}{
		{"no neighbour named", made[0].ID, "", "", mwanachamataskmanager.ErrInvalidPlacement},
		{"beside itself", made[0].ID, made[0].ID, "", mwanachamataskmanager.ErrInvalidPlacement},
		{"unknown card", "nope", made[0].ID, "", mwanachamataskmanager.ErrTaskNotFound},
		{"unknown neighbour", made[0].ID, "nope", "", mwanachamataskmanager.ErrTaskNotFound},
		{"deleted neighbour", made[0].ID, made[2].ID, "", mwanachamataskmanager.ErrTaskNotFound},
	}
	for _, c := range cases {
		if _, err := mgr.PlaceTask(ctx, c.task, c.after, c.before); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
	third := createTitled(t, mgr, "d")[0]
	if _, err := mgr.PlaceTask(ctx, third.ID, made[1].ID, made[0].ID); !errors.Is(err, mwanachamataskmanager.ErrInvalidPlacement) {
		t.Errorf("neighbours the wrong way round: err = %v", err)
	}
	if got := listedTitles(t, mgr); got != "a,b,d" {
		t.Fatalf("a refused placement changed the board: %s", got)
	}
}

func TestAProjectListsItsCardsInRankOrder(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	project, _ := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Sprint"})
	made := createTitled(t, mgr, "a", "b", "c")
	for _, task := range made {
		if err := mgr.AddTaskToProject(ctx, task.ID, project.ID); err != nil {
			t.Fatalf("AddTaskToProject: %v", err)
		}
	}
	if _, err := mgr.PlaceTask(ctx, made[2].ID, "", made[0].ID); err != nil {
		t.Fatalf("PlaceTask: %v", err)
	}
	listed, err := mgr.ListTasksInProject(ctx, project.ID)
	if err != nil {
		t.Fatalf("ListTasksInProject: %v", err)
	}
	var titles []string
	for _, task := range listed {
		titles = append(titles, task.Title)
	}
	if got := strings.Join(titles, ","); got != "c,a,b" {
		t.Fatalf("list = %s", got)
	}
}
