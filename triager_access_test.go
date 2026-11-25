package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const triagerAccount = "triager"

func restrictedHubWithOneIssue(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr := restrictedManager(t)
	if _, err := giveAccess(mgr, context.Background(), "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	softwareTask(t, mgr)
	return mgr
}

func issueCountFor(t *testing.T, mgr mwanachamataskmanager.TaskManager, requester string) int {
	t.Helper()
	tasks, err := mgr.ListTasks(context.Background(), mwanachamataskmanager.TaskFilter{RequesterID: requester})
	if err != nil {
		t.Fatalf("ListTasks(%q): %v", requester, err)
	}
	return len(tasks)
}

func TestTriagerAccess_ACallerWithAnAccountIdInNoGroupSeesNothingOnceGrantsExist(t *testing.T) {
	mgr := restrictedHubWithOneIssue(t)

	if got := issueCountFor(t, mgr, triagerAccount); got != 0 {
		t.Fatalf("the Triager account in no group lists %d issues, want 0", got)
	}
	if got := visibleIDs(t, mgr, triagerAccount); len(got) != 0 {
		t.Fatalf("the Triager account in no group sees backlogs %v, want none", got)
	}
	_, err := mgr.CreateTaskAs(context.Background(), triagerAccount, mwanachamataskmanager.Task{Title: "x", Backlog: "software", Step: "open"})
	wantForbidden(t, "Triager create", err)
}

func TestTriagerAccess_ACallWithNoAccountIdIsNotNarrowed(t *testing.T) {
	mgr := restrictedHubWithOneIssue(t)

	if got := issueCountFor(t, mgr, ""); got != 1 {
		t.Fatalf("a call with no account id lists %d issues, want 1", got)
	}
	if got := visibleIDs(t, mgr, ""); !equalIDs(got, []string{"backlog", "software"}) {
		t.Fatalf("a call with no account id sees backlogs %v", got)
	}
}

func TestTriagerAccess_SeatingTheTriagerInTheOwnerGroupOpensEverything(t *testing.T) {
	mgr := restrictedHubWithOneIssue(t)
	seatTriagerInOwnerGroup(mgr)

	if got := issueCountFor(t, mgr, triagerAccount); got != 1 {
		t.Fatalf("the Triager in the owner group lists %d issues, want 1", got)
	}
	if _, err := mgr.CreateTaskAs(context.Background(), triagerAccount, mwanachamataskmanager.Task{Title: "y", Backlog: "software", Step: "open"}); err != nil {
		t.Fatalf("the Triager in the owner group creates: %v", err)
	}
}

func seatTriagerInOwnerGroup(mgr mwanachamataskmanager.TaskManager) {
	mgr.UseGroups(func(_ context.Context, id string) ([]string, error) {
		if id == triagerAccount {
			return []string{ownerGroup}, nil
		}
		return nil, nil
	})
	fixture := fixtureGroupTags()
	mgr.UseGroupTags(func(ctx context.Context, id string) ([]string, error) {
		if id == triagerAccount {
			return []string{"all"}, nil
		}
		return fixture(ctx, id)
	})
}
