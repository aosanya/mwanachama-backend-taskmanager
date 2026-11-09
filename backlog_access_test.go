package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const (
	ownerGroup = "g-owner"
	devGroup   = "g-dev"
	docsGroup  = "g-docs"
)

func groupsOfPeople() mwanachamataskmanager.Groups {
	byPerson := map[string][]string{
		"owner": {ownerGroup},
		"dev":   {devGroup},
		"both":  {devGroup, docsGroup},
		"none":  nil,
	}
	return func(_ context.Context, id string) ([]string, error) { return byPerson[id], nil }
}

func managerWithTwoBacklogs(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr := newTestManager(t)
	mgr.UseGroups(groupsOfPeople())
	mgr.UseGroupExists(knownGroups(ownerGroup, devGroup, docsGroup))
	if _, err := mgr.CreateBacklog(context.Background(), "Software", []string{"Open", "Shipped"}); err != nil {
		t.Fatal(err)
	}
	return mgr
}

func visibleIDs(t *testing.T, mgr mwanachamataskmanager.TaskManager, person string) []string {
	t.Helper()
	list, err := mgr.ListBacklogsFor(context.Background(), person)
	if err != nil {
		t.Fatalf("ListBacklogsFor(%s): %v", person, err)
	}
	ids := make([]string, len(list.Items))
	for i, b := range list.Items {
		ids[i] = b.ID
	}
	return ids
}

func TestBacklogAccess_OpenUntilAnOwnerGroupOrAGrantIsSet(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	for _, person := range []string{"owner", "dev", "none", ""} {
		if got := visibleIDs(t, mgr, person); !equalIDs(got, []string{"backlog", "software"}) {
			t.Fatalf("%q sees %v before any access is set", person, got)
		}
	}
}

func TestBacklogAccess_OwnerGroupSeesEveryBacklogAndANewBacklogStartsWithItAlone(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	if got := visibleIDs(t, mgr, "owner"); !equalIDs(got, []string{"backlog", "software"}) {
		t.Fatalf("owner sees %v", got)
	}
	for _, person := range []string{"dev", "none"} {
		if got := visibleIDs(t, mgr, person); len(got) != 0 {
			t.Fatalf("%s sees %v", person, got)
		}
	}
	if _, err := mgr.CreateBacklog(ctx, "Design", []string{"Open"}); err != nil {
		t.Fatal(err)
	}
	if got := visibleIDs(t, mgr, "dev"); len(got) != 0 {
		t.Fatalf("dev sees %v in a new backlog", got)
	}
	if got := visibleIDs(t, mgr, "owner"); len(got) != 3 {
		t.Fatalf("owner sees %v", got)
	}
}

func TestBacklogAccess_AGroupSeesOnlyTheBacklogsItWasGiven(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	mgr.SetBacklogOwnerGroup(ctx, ownerGroup)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}
	if got := visibleIDs(t, mgr, "dev"); !equalIDs(got, []string{"software"}) {
		t.Fatalf("dev sees %v", got)
	}
	if got := visibleIDs(t, mgr, "both"); !equalIDs(got, []string{"software"}) {
		t.Fatalf("both sees %v", got)
	}
	if got := visibleIDs(t, mgr, "none"); len(got) != 0 {
		t.Fatalf("none sees %v", got)
	}
}

func TestBacklogAccess_GivingAgainChangesTheLevelAndTakingRemovesIt(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	mgr.SetBacklogOwnerGroup(ctx, ownerGroup)
	mgr.GiveBacklogAccess(ctx, "software", devGroup, "see")
	got, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "move")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || got.Groups[0].Level != mwanachamataskmanager.AccessMove {
		t.Fatalf("got %+v", got)
	}
	got, err = mgr.TakeBacklogAccess(ctx, "software", devGroup)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestBacklogAccess_RefusesAnUnknownLevelBacklogOrEmptyGroup(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	mgr.SetBacklogOwnerGroup(ctx, ownerGroup)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "edit"); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("level: %v", err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "software", " ", "see"); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("group: %v", err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "nope", devGroup, "see"); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("backlog: %v", err)
	}
	if _, err := mgr.ListBacklogAccess(ctx, "nope"); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("list: %v", err)
	}
}

func TestBacklogAccess_MovingNeedsCanMoveOnBothBacklogs(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	mgr.SetBacklogOwnerGroup(ctx, ownerGroup)
	task := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	mgr.GiveBacklogAccess(ctx, "software", devGroup, "move")
	if _, err := mgr.MoveTaskToStepAs(ctx, "dev", task.ID, "software", "open"); !errors.Is(err, mwanachamataskmanager.ErrBacklogForbidden) {
		t.Fatalf("dev moves out of a backlog it cannot see: %v", err)
	}
	if _, err := mgr.MoveTaskToStepAs(ctx, "owner", task.ID, "software", "open"); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if _, err := mgr.MoveTaskToStepAs(ctx, "dev", task.ID, "software", "shipped"); err != nil {
		t.Fatalf("dev with Can move: %v", err)
	}
	mgr.GiveBacklogAccess(ctx, "software", devGroup, "see")
	if _, err := mgr.MoveTaskToStepAs(ctx, "dev", task.ID, "software", "open"); !errors.Is(err, mwanachamataskmanager.ErrBacklogForbidden) {
		t.Fatalf("dev with Can see: %v", err)
	}
}

func TestBacklogAccess_CallsWithoutAPersonAreNotNarrowed(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	mgr.SetBacklogOwnerGroup(ctx, ownerGroup)
	task := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if got := visibleIDs(t, mgr, ""); len(got) != 2 {
		t.Fatalf("got %v", got)
	}
	if _, err := mgr.MoveTaskToStepAs(ctx, "", task.ID, "software", "open"); err != nil {
		t.Fatal(err)
	}
}

func TestBacklogAccess_DeletingABacklogDropsItsGrants(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	mgr.SetBacklogOwnerGroup(ctx, ownerGroup)
	mgr.GiveBacklogAccess(ctx, "software", devGroup, "move")
	if err := mgr.DeleteBacklog(ctx, "software"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateBacklog(ctx, "Software", []string{"Open"}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListBacklogAccess(ctx, "software")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 0 {
		t.Fatalf("a new backlog of the same name inherits %+v", got.Groups)
	}
}
