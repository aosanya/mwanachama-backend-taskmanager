package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func notRefusedByAccess(t *testing.T, what string, err error) {
	t.Helper()
	if errors.Is(err, mwanachamataskmanager.ErrBacklogForbidden) {
		t.Fatalf("%s: a person with Can move was refused by the backlog's access: %v", what, err)
	}
}

func TestBacklogWrites_AssigningAndLinkingAnIssueNeedsCanMoveOnItsBacklog(t *testing.T) {
	ctx := context.Background()
	hub := sprintWithOneTaskDevCanSee(t)
	actor := mwanachamataskmanager.AssigneeKind("actor")

	wantForbidden(t, "assign with Can see", hub.mgr.AssignTaskAs(ctx, "dev", hub.shown.ID, "agent-1", ""))
	wantForbidden(t, "assign to a holder with Can see", hub.mgr.AssignTaskToHolderAs(ctx, "dev", hub.shown.ID, actor, "someone"))
	wantForbidden(t, "link to a run with Can see", hub.mgr.LinkTaskToRunAs(ctx, "dev", "run-1", hub.shown.ID))
	wantForbidden(t, "link to a case with Can see", hub.mgr.LinkCaseTaskAs(ctx, "dev", "case-1", hub.shown.ID))
	wantForbidden(t, "assign an issue in a hidden backlog", hub.mgr.AssignTaskAs(ctx, "dev", hub.hidden.ID, "agent-1", ""))

	if _, err := hub.mgr.GiveBacklogAccess(ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	notRefusedByAccess(t, "assign", hub.mgr.AssignTaskAs(ctx, "dev", hub.shown.ID, "agent-1", ""))
	notRefusedByAccess(t, "assign to a holder", hub.mgr.AssignTaskToHolderAs(ctx, "dev", hub.shown.ID, actor, "someone"))
	notRefusedByAccess(t, "link to a run", hub.mgr.LinkTaskToRunAs(ctx, "dev", "run-1", hub.shown.ID))
	notRefusedByAccess(t, "link to a case", hub.mgr.LinkCaseTaskAs(ctx, "dev", "case-1", hub.shown.ID))
	wantForbidden(t, "Can move on software does not reach the first backlog", hub.mgr.AssignTaskAs(ctx, "dev", hub.hidden.ID, "agent-1", ""))
}

func TestBacklogWrites_TheOwnerGroupIsNeverNarrowedOnAssignment(t *testing.T) {
	ctx := context.Background()
	hub := sprintWithOneTaskDevCanSee(t)

	notRefusedByAccess(t, "owner assigns an issue in any backlog", hub.mgr.AssignTaskAs(ctx, "owner", hub.hidden.ID, "agent-1", ""))
	notRefusedByAccess(t, "a call with no person", hub.mgr.UnassignTaskAs(ctx, "", hub.hidden.ID))
}
