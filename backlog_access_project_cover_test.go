package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type sprintHub struct {
	mgr     mwanachamataskmanager.TaskManager
	project mwanachamataskmanager.Project
	shown   mwanachamataskmanager.Task
	hidden  mwanachamataskmanager.Task
}

func sprintWithOneTaskDevCanSee(t *testing.T) sprintHub {
	t.Helper()
	ctx := context.Background()
	mgr := restrictedManager(t)
	project, err := mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Sprint"})
	if err != nil {
		t.Fatal(err)
	}
	shown, err := mgr.CreateTaskInProject(ctx, project.ProjectName, mwanachamataskmanager.Task{Title: "in software", Backlog: "software", Step: "open"})
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := mgr.CreateTaskInProject(ctx, project.ProjectName, mwanachamataskmanager.Task{Title: "in the first backlog"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := giveSee(mgr, ctx, "software", devGroup); err != nil {
		t.Fatal(err)
	}
	return sprintHub{mgr: mgr, project: project, shown: shown, hidden: hidden}
}

func TestBacklogReads_AProjectListAndANameLookupHideWhatThePersonCannotSee(t *testing.T) {
	ctx := context.Background()
	hub := sprintWithOneTaskDevCanSee(t)

	forDev, err := hub.mgr.ListTasksInProjectFor(ctx, "dev", hub.project.ID)
	if err != nil || len(forDev) != 1 || forDev[0].ID != hub.shown.ID {
		t.Fatalf("dev sees %+v %v, want only the software issue", forDev, err)
	}
	forOwner, err := hub.mgr.ListTasksInProjectFor(ctx, "owner", hub.project.ID)
	if err != nil || len(forOwner) != 2 {
		t.Fatalf("owner sees %d issues, want both: %v", len(forOwner), err)
	}

	got, err := hub.mgr.GetTaskByNameFor(ctx, "dev", hub.project.ProjectName, hub.shown.TaskName)
	if err != nil || got.ID != hub.shown.ID {
		t.Fatalf("dev reads the software issue by name: %+v %v", got, err)
	}
	_, err = hub.mgr.GetTaskByNameFor(ctx, "dev", hub.project.ProjectName, hub.hidden.TaskName)
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("a hidden issue read by name: got %v, want ErrTaskNotFound", err)
	}
	if got, err := hub.mgr.GetTaskByNameFor(ctx, "owner", hub.project.ProjectName, hub.hidden.TaskName); err != nil || got.ID != hub.hidden.ID {
		t.Fatalf("owner reads the hidden issue by name: %+v %v", got, err)
	}
}

func TestBacklogReads_AssignmentsOfAnIssueAreHiddenWithTheIssue(t *testing.T) {
	ctx := context.Background()
	hub := sprintWithOneTaskDevCanSee(t)

	if _, err := hub.mgr.ListTaskAssignmentsFor(ctx, "dev", hub.shown.ID); err != nil {
		t.Fatalf("dev lists the assignments of a visible issue: %v", err)
	}
	_, err := hub.mgr.ListTaskAssignmentsFor(ctx, "dev", hub.hidden.ID)
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("assignments of a hidden issue: got %v, want ErrTaskNotFound", err)
	}
}

func TestBacklogReads_AnEdgeToAnIssueThePersonCannotSeeIsLeftOut(t *testing.T) {
	ctx := context.Background()
	hub := sprintWithOneTaskDevCanSee(t)
	other := softwareTask(t, hub.mgr)
	visibleEdge := mwanachamataskmanager.Relationship{Label: "depends_on", FromID: hub.shown.ID, ToID: other.ID}
	hiddenEdge := mwanachamataskmanager.Relationship{Label: "depends_on", FromID: hub.shown.ID, ToID: hub.hidden.ID}
	for _, edge := range []mwanachamataskmanager.Relationship{visibleEdge, hiddenEdge} {
		if _, err := hub.mgr.CreateRelationshipAs(ctx, "owner", edge); err != nil {
			t.Fatal(err)
		}
	}

	forOwner, err := hub.mgr.TraverseRelationshipsFor(ctx, "owner", hub.shown.ID, "depends_on", mwanachamataskmanager.DirectionOutbound)
	if err != nil || len(forOwner) != 2 {
		t.Fatalf("owner sees %d edges, want 2: %v", len(forOwner), err)
	}
	forDev, err := hub.mgr.TraverseRelationshipsFor(ctx, "dev", hub.shown.ID, "depends_on", mwanachamataskmanager.DirectionOutbound)
	if err != nil || len(forDev) != 1 || forDev[0].ToID != other.ID {
		t.Fatalf("dev sees %+v %v, want only the edge between two visible issues", forDev, err)
	}
	fromHidden, err := hub.mgr.TraverseRelationshipsFor(ctx, "dev", hub.hidden.ID, "depends_on", mwanachamataskmanager.DirectionInbound)
	if err != nil || len(fromHidden) != 0 {
		t.Fatalf("dev walks edges of a hidden issue: %+v %v", fromHidden, err)
	}
}

func TestBacklogWrites_ProjectMembershipAndAssignmentNeedCanMoveOnTheIssue(t *testing.T) {
	ctx := context.Background()
	hub := sprintWithOneTaskDevCanSee(t)
	other, err := hub.mgr.CreateProject(ctx, mwanachamataskmanager.Project{Name: "Other"})
	if err != nil {
		t.Fatal(err)
	}

	wantForbidden(t, "add to a project with Can see", hub.mgr.AddTaskToProjectAs(ctx, "dev", hub.shown.ID, other.ID))
	wantForbidden(t, "remove from a project with Can see", hub.mgr.RemoveTaskFromProjectAs(ctx, "dev", hub.shown.ID, hub.project.ID))
	wantForbidden(t, "unassign with Can see", hub.mgr.UnassignTaskAs(ctx, "dev", hub.shown.ID))
	_, err = hub.mgr.UnblockTaskAs(ctx, "dev", hub.shown.ID, "note")
	wantForbidden(t, "unblock with Can see", err)
	wantForbidden(t, "unblock dependents with Can see", hub.mgr.UnblockDependentsAs(ctx, "dev", hub.shown.ID))
	_, err = hub.mgr.CreateTaskInProjectAs(ctx, "dev", hub.project.ProjectName, mwanachamataskmanager.Task{Title: "x", Backlog: "software", Step: "open"})
	wantForbidden(t, "create in a project with Can see", err)

	if _, err := giveAccess(hub.mgr, ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	if err := hub.mgr.AddTaskToProjectAs(ctx, "dev", hub.shown.ID, other.ID); err != nil {
		t.Fatalf("add to a project with Can move: %v", err)
	}
	if members, err := hub.mgr.ListTasksInProjectFor(ctx, "dev", other.ID); err != nil || len(members) != 1 || members[0].ID != hub.shown.ID {
		t.Fatalf("members of the second project: %+v %v", members, err)
	}
	if err := hub.mgr.RemoveTaskFromProjectAs(ctx, "dev", hub.shown.ID, other.ID); err != nil {
		t.Fatalf("remove from a project with Can move: %v", err)
	}
	if members, err := hub.mgr.ListTasksInProjectFor(ctx, "dev", other.ID); err != nil || len(members) != 0 {
		t.Fatalf("members after removing: %+v %v", members, err)
	}
	created, err := hub.mgr.CreateTaskInProjectAs(ctx, "dev", hub.project.ProjectName, mwanachamataskmanager.Task{Title: "made by dev", Backlog: "software", Step: "open"})
	if err != nil || created.Backlog != "software" || created.TaskName == "" {
		t.Fatalf("create in a project with Can move: %+v %v", created, err)
	}
	_, err = hub.mgr.CreateTaskInProjectAs(ctx, "dev", hub.project.ProjectName, mwanachamataskmanager.Task{Title: "unplaced"})
	wantForbidden(t, "an unplaced issue lands in the first backlog, which dev cannot move in", err)
	wantForbidden(t, "move on another backlog's issue", hub.mgr.AddTaskToProjectAs(ctx, "dev", hub.hidden.ID, other.ID))
}
