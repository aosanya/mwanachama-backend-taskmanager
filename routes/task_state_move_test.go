package routes_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func w31Task(t *testing.T, tm mwanachamataskmanager.TaskManager) mwanachamataskmanager.Task {
	t.Helper()
	task, err := tm.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestTheAgencysStatesAreReadInOrderForTheBoardsColumns(t *testing.T) {
	m := w73Mux(wk11Manager(t))
	rec := w73Do(t, m, "staff-1", "GET", "/task-states", nil)
	var got mwanachamataskmanager.TaskStateList
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("states %d %s", rec.Code, rec.Body)
	}
	var names []string
	for _, s := range got.Items {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "To do,Doing,Waiting,Done,Cancelled" {
		t.Fatalf("names %v", names)
	}
}

func TestMovingToDoneWalksThroughDoing(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)
	task := w31Task(t, tm)
	rec := w73Do(t, m, "staff-1", "POST", "/tasks/"+task.ID+"/move", map[string]any{"state": "Done"})
	var moved mwanachamataskmanager.Task
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &moved) != nil {
		t.Fatalf("move %d %s", rec.Code, rec.Body)
	}
	if moved.Status != mwanachamataskmanager.TaskStatusCompleted || moved.State != "Done" {
		t.Fatalf("moved %+v", moved)
	}
}

func TestMovingIntoTheStateATaskIsInLeavesIt(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)
	task := w31Task(t, tm)
	rec := w73Do(t, m, "staff-1", "POST", "/tasks/"+task.ID+"/move", map[string]any{"state": "To do"})
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"pending"`) {
		t.Fatalf("move %d %s", rec.Code, rec.Body)
	}
}

func TestAMoveOutOfATerminalStatusIsRefusedWithTheReason(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)
	task := w31Task(t, tm)
	if rec := w73Do(t, m, "staff-1", "POST", "/tasks/"+task.ID+"/move", map[string]any{"state": "Cancelled"}); rec.Code != 200 {
		t.Fatalf("cancel %d %s", rec.Code, rec.Body)
	}
	rec := w73Do(t, m, "staff-1", "POST", "/tasks/"+task.ID+"/move", map[string]any{"state": "Doing"})
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "cannot reach") {
		t.Fatalf("refused %d %s", rec.Code, rec.Body)
	}
}

func TestAMoveToAStateTheAgencyDoesNotHaveIsRefused(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)
	task := w31Task(t, tm)
	rec := w73Do(t, m, "staff-1", "POST", "/tasks/"+task.ID+"/move", map[string]any{"state": "Archived"})
	if rec.Code != 400 {
		t.Fatalf("unknown %d %s", rec.Code, rec.Body)
	}
}

func TestAMoveOfAMissingTaskIsNotFound(t *testing.T) {
	m := w73Mux(wk11Manager(t))
	rec := w73Do(t, m, "staff-1", "POST", "/tasks/nope/move", map[string]any{"state": "Done"})
	if rec.Code != 404 {
		t.Fatalf("missing %d %s", rec.Code, rec.Body)
	}
}
