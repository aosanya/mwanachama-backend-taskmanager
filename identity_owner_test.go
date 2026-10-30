package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aosanya/mwanachama-backend-shared/identity"
	"github.com/aosanya/mwanachama-backend-shared/recur"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

var _ identity.Owner = mwanachamataskmanager.TaskManager(nil)

func seedOwnedThings(t *testing.T) (mwanachamataskmanager.TaskManager, map[string]string) {
	t.Helper()
	ctx := context.Background()
	mgr, db, tables := newTestManagerWithDB(t, nil)

	task, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Draft the minutes"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	project, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Annual meeting"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	agent, err := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "scribe-1"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	opened, err := mgr.OpenCase(ctx, "resident-1", mwanachamataskmanager.Case{Title: "Broken tap"})
	if err != nil {
		t.Fatalf("OpenCase: %v", err)
	}
	run, err := mgr.CreateWorkflowRun(ctx, "onboarding", "member.joined", "system")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	todo, err := mgr.CreateTaskTodo(ctx, mwanachamataskmanager.TaskTodo{Title: "Book the hall", Instructions: "ring them", ParentTaskID: task.ID, Ordinality: 1})
	if err != nil {
		t.Fatalf("CreateTaskTodo: %v", err)
	}
	rule, err := recur.ParseRule("FREQ=MONTHLY")
	if err != nil {
		t.Fatal(err)
	}
	repeating, err := mgr.CreateRepeatingTask(ctx, mwanachamataskmanager.RepeatingTask{
		Title:        "Margin review",
		Schedule:     recur.Schedule{Start: time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC), Zone: time.UTC, Rule: &rule},
		AssigneeKind: models.AssigneeActor,
		AssigneeID:   "actor-7",
	})
	if err != nil {
		t.Fatalf("CreateRepeatingTask: %v", err)
	}
	criteriaID := "criteria-1"
	if err := db.Table(tables.AcceptanceCriteria).Create(map[string]any{"id": criteriaID, "code": "AC-1", "title": "Signed off", "parent_id": task.ID}).Error; err != nil {
		t.Fatalf("insert criteria: %v", err)
	}
	return mgr, map[string]string{
		"task": task.ID, "project": project.ID, "agent": agent.ID, "case": opened.ID,
		"workflow_run": run.ID, "task_todo": todo.ID, "repeating_task": repeating.ID,
		"acceptance_criteria": criteriaID,
	}
}

func TestStableIdentityIsTheRawTaskmanagerInstanceUuidOfEachObject(t *testing.T) {
	mgr, ids := seedOwnedThings(t)
	ctx := context.Background()
	for object, id := range ids {
		got, err := mgr.StableIdentity(ctx, object, id)
		if err != nil {
			t.Fatalf("StableIdentity(%s): %v", object, err)
		}
		if want := "taskmanager/test/" + id; got != want {
			t.Errorf("StableIdentity(%s) = %q, want %q", object, got, want)
		}
	}
}

func TestResolveGivesBackTheObjectAndIdStableIdentityWasAskedFor(t *testing.T) {
	mgr, ids := seedOwnedThings(t)
	ctx := context.Background()
	for object, id := range ids {
		raw, err := mgr.StableIdentity(ctx, object, id)
		if err != nil {
			t.Fatalf("StableIdentity(%s): %v", object, err)
		}
		gotObject, gotID, err := mgr.Resolve(ctx, raw)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", raw, err)
		}
		if gotObject != object || gotID != id {
			t.Errorf("Resolve(%q) = (%q, %q), want (%q, %q)", raw, gotObject, gotID, object, id)
		}
	}
}

func TestADeletedTaskProjectOrStepIsAConfirmedMissNotAnIdentity(t *testing.T) {
	mgr, ids := seedOwnedThings(t)
	ctx := context.Background()
	raws := map[string]string{}
	for _, object := range []string{"task", "project"} {
		raw, err := mgr.StableIdentity(ctx, object, ids[object])
		if err != nil {
			t.Fatalf("StableIdentity(%s): %v", object, err)
		}
		raws[object] = raw
	}
	if err := mgr.DeleteTask(ctx, ids["task"]); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if err := mgr.DeleteProject(ctx, ids["project"]); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	for object, raw := range raws {
		if _, _, err := mgr.Resolve(ctx, raw); !errors.Is(err, identity.ErrNotFound) {
			t.Errorf("Resolve of a deleted %s: want ErrNotFound, got %v", object, err)
		}
		if _, err := mgr.StableIdentity(ctx, object, ids[object]); !errors.Is(err, identity.ErrNotFound) {
			t.Errorf("StableIdentity of a deleted %s: want ErrNotFound, got %v", object, err)
		}
	}
}

func TestResolveNamesAMissAndARawItDoesNotOwnApart(t *testing.T) {
	mgr, ids := seedOwnedThings(t)
	ctx := context.Background()
	cases := []struct {
		raw  string
		want error
	}{
		{"taskmanager/test/00000000-0000-0000-0000-000000000000", identity.ErrNotFound},
		{"git/test/" + ids["task"], identity.ErrNotOwned},
		{"taskmanager/elsewhere/" + ids["task"], identity.ErrNotOwned},
		{"taskmanager/test", identity.ErrInvalidRaw},
		{"", identity.ErrInvalidRaw},
	}
	for _, c := range cases {
		if _, _, err := mgr.Resolve(ctx, c.raw); !errors.Is(err, c.want) {
			t.Errorf("Resolve(%q): want %v, got %v", c.raw, c.want, err)
		}
	}
}

func TestStableIdentityNamesAMissAndAnObjectWithNoIdentityApart(t *testing.T) {
	mgr, ids := seedOwnedThings(t)
	ctx := context.Background()
	if _, err := mgr.StableIdentity(ctx, "task", "no-such-row"); !errors.Is(err, identity.ErrNotFound) {
		t.Errorf("a missing row: want ErrNotFound, got %v", err)
	}
	for _, object := range []string{"blocker", "membership", "case_vote", "tag", "no_such_object"} {
		if _, err := mgr.StableIdentity(ctx, object, ids["task"]); !errors.Is(err, mwanachamataskmanager.ErrNoStableIdentity) {
			t.Errorf("%s: want ErrNoStableIdentity, got %v", object, err)
		}
	}
}
