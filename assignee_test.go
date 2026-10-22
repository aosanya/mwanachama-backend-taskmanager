package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestAssignTaskToHolder_PersonAndRoleAreListedForTheirHolders(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	personal, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	forRole, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	elsewhere, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})

	if err := mgr.AssignTaskToHolder(ctx, personal.ID, models.AssigneeActor, "actor-1"); err != nil {
		t.Fatalf("assign actor: %v", err)
	}
	if err := mgr.AssignTaskToHolder(ctx, forRole.ID, models.AssigneeRole, "role-desk"); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	if err := mgr.AssignTaskToHolder(ctx, elsewhere.ID, models.AssigneeRole, "role-other"); err != nil {
		t.Fatalf("assign other role: %v", err)
	}

	got, err := mgr.ListHeldTasks(ctx, mwanachamataskmanager.HolderTasksQuery{ActorID: "actor-1", RoleKindIDs: []string{"role-desk"}})
	if err != nil {
		t.Fatalf("ListHeldTasks: %v", err)
	}
	ids := map[string]bool{}
	for _, task := range got {
		ids[task.ID] = true
	}
	if len(got) != 2 || !ids[personal.ID] || !ids[forRole.ID] {
		t.Fatalf("held tasks = %v, want the personal and the role's task", ids)
	}

	second, _ := mgr.ListHeldTasks(ctx, mwanachamataskmanager.HolderTasksQuery{ActorID: "actor-2", RoleKindIDs: []string{"role-desk"}})
	if len(second) != 1 || second[0].ID != forRole.ID {
		t.Fatalf("another holder of the role should see its task, got %v", second)
	}

	none, _ := mgr.ListHeldTasks(ctx, mwanachamataskmanager.HolderTasksQuery{})
	if len(none) != 0 {
		t.Fatalf("an empty query lists nothing, got %d", len(none))
	}
}

func TestAssignTaskToHolder_ActorIDDoesNotMatchARoleWithTheSameID(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	_ = mgr.AssignTaskToHolder(ctx, task.ID, models.AssigneeRole, "same-id")

	got, _ := mgr.ListHeldTasks(ctx, mwanachamataskmanager.HolderTasksQuery{ActorID: "same-id"})
	if len(got) != 0 {
		t.Fatalf("a person must not pick up a role's task by sharing its id, got %d", len(got))
	}
}

func TestAssignTaskToHolder_RefusesAnUnknownKindOrEmptyID(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	for _, c := range []struct {
		kind mwanachamataskmanager.AssigneeKind
		id   string
	}{{"agent", "a"}, {"team", "t"}, {models.AssigneeActor, ""}} {
		if err := mgr.AssignTaskToHolder(ctx, task.ID, c.kind, c.id); !errors.Is(err, mwanachamataskmanager.ErrInvalidAssignee) {
			t.Errorf("kind %q id %q: got %v, want ErrInvalidAssignee", c.kind, c.id, err)
		}
	}
	if err := mgr.AssignTaskToHolder(ctx, "missing", models.AssigneeActor, "a"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Errorf("missing task: got %v", err)
	}
}

func TestUnassignTask_KeepsTheHistoryOfWhoHeldIt(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, mwanachamataskmanager.Task{})
	agent, _ := mgr.UpsertAgent(ctx, mwanachamataskmanager.Agent{AgentID: "bot"})

	if err := mgr.AssignTask(ctx, task.ID, agent.ID, ""); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	if err := mgr.AssignTaskToHolder(ctx, task.ID, models.AssigneeRole, "role-desk"); err != nil {
		t.Fatalf("AssignTaskToHolder: %v", err)
	}
	if err := mgr.UnassignTask(ctx, task.ID); err != nil {
		t.Fatalf("UnassignTask: %v", err)
	}

	history, err := mgr.ListTaskAssignments(ctx, task.ID)
	if err != nil {
		t.Fatalf("ListTaskAssignments: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history = %d stretches, want 2", len(history))
	}
	if history[0].AssigneeKind != models.AssigneeAgent || history[0].AssigneeID != agent.ID || history[0].UnassignedAt == "" {
		t.Errorf("first stretch = %+v", history[0])
	}
	if history[1].AssigneeKind != models.AssigneeRole || history[1].AssigneeID != "role-desk" || history[1].UnassignedAt == "" {
		t.Errorf("second stretch = %+v", history[1])
	}

	got, _ := mgr.GetTask(ctx, task.ID)
	if got.AssignedTo != "" || got.AssigneeKind != "" {
		t.Errorf("task still assigned: %q %q", got.AssignedTo, got.AssigneeKind)
	}
	held, _ := mgr.ListHeldTasks(ctx, mwanachamataskmanager.HolderTasksQuery{RoleKindIDs: []string{"role-desk"}})
	if len(held) != 0 {
		t.Errorf("an unassigned task is still listed for the role")
	}
}
