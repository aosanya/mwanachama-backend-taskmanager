package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func triagerSeatedInOwnerGroup(t *testing.T) (mwanachamataskmanager.TaskManager, mwanachamataskmanager.Task) {
	t.Helper()
	mgr := restrictedManager(t)
	task := softwareTask(t, mgr)
	seatTriagerInOwnerGroup(mgr)
	return mgr, task
}

func TestTriagerInOwnerGroup_ReadsAnIssueNoOtherGroupHasAGrantOn(t *testing.T) {
	mgr, task := triagerSeatedInOwnerGroup(t)

	got, err := mgr.GetTaskFor(context.Background(), triagerAccount, task.ID)
	if err != nil || got.ID != task.ID {
		t.Fatalf("the Triager reads the issue: %v, %v", got.ID, err)
	}
	if ids := visibleIDs(t, mgr, triagerAccount); !equalIDs(ids, []string{"backlog", "software"}) {
		t.Fatalf("the Triager sees backlogs %v", ids)
	}
}

func TestTriagerInOwnerGroup_MovesAnIssueToTheNextStep(t *testing.T) {
	mgr, task := triagerSeatedInOwnerGroup(t)

	moved, err := mgr.MoveTaskToStepAs(context.Background(), triagerAccount, task.ID, "software", "shipped")
	if err != nil {
		t.Fatalf("the Triager moves the issue: %v", err)
	}
	if moved.Step != "shipped" {
		t.Fatalf("the issue is at %q, want shipped", moved.Step)
	}
}

func TestTriagerInOwnerGroup_OtherAccountsInNoGroupStillGetNothing(t *testing.T) {
	mgr, task := triagerSeatedInOwnerGroup(t)

	if _, err := mgr.GetTaskFor(context.Background(), "someone-else", task.ID); err == nil {
		t.Fatal("an account in no group reads the issue")
	}
	_, err := mgr.MoveTaskToStepAs(context.Background(), "someone-else", task.ID, "software", "shipped")
	wantForbidden(t, "account in no group moves an issue", err)
}
