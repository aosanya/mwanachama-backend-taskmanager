package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func knownGroups(ids ...string) mwanachamataskmanager.GroupExists {
	known := map[string]bool{}
	for _, id := range ids {
		known[id] = true
	}
	return func(_ context.Context, id string) (bool, error) { return known[id], nil }
}

func restrictedManager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr := managerWithTwoBacklogs(t)
	mgr.UseGroupExists(knownGroups(ownerGroup, devGroup, docsGroup))
	ctx := context.Background()
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	return mgr
}

func softwareTask(t *testing.T, mgr mwanachamataskmanager.TaskManager) mwanachamataskmanager.Task {
	t.Helper()
	return createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "in software", Backlog: "software", Step: "open"})
}

func wantForbidden(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, mwanachamataskmanager.ErrBacklogForbidden) {
		t.Fatalf("%s: got %v, want ErrBacklogForbidden", what, err)
	}
}

func TestBacklogWrites_CanSeeDoesNotAllowWriting(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	task := softwareTask(t, mgr)
	other := softwareTask(t, mgr)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}

	_, err := mgr.CreateTaskAs(ctx, "dev", mwanachamataskmanager.Task{Title: "x", Backlog: "software", Step: "open"})
	wantForbidden(t, "create", err)
	renamed := task
	renamed.Title = "renamed"
	_, err = mgr.UpdateTaskAs(ctx, "dev", renamed)
	wantForbidden(t, "update", err)
	_, err = mgr.MoveTaskToStateAs(ctx, "dev", task.ID, "Done")
	wantForbidden(t, "state move", err)
	_, err = mgr.PlaceTaskAs(ctx, "dev", task.ID, other.ID, "")
	wantForbidden(t, "place", err)
	wantForbidden(t, "delete", mgr.DeleteTaskAs(ctx, "dev", task.ID))

	if got, err := mgr.GetTask(ctx, task.ID); err != nil || got.Title != "in software" || got.Status != mwanachamataskmanager.TaskStatusPending {
		t.Fatalf("a refused write changed the task: %+v %v", got, err)
	}
}

func TestBacklogWrites_CanMoveAllowsWritingAndNoPersonIsNotNarrowed(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	task := softwareTask(t, mgr)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	renamed := task
	renamed.Title = "renamed"
	if _, err := mgr.UpdateTaskAs(ctx, "dev", renamed); err != nil {
		t.Fatalf("dev with Can move: %v", err)
	}
	if _, err := mgr.CreateTaskAs(ctx, "dev", mwanachamataskmanager.Task{Title: "y", Backlog: "software", Step: "open"}); err != nil {
		t.Fatalf("dev creates with Can move: %v", err)
	}
	if _, err := mgr.CreateTaskAs(ctx, "", mwanachamataskmanager.Task{Title: "loader"}); err != nil {
		t.Fatalf("no person: %v", err)
	}
	if _, err := mgr.CreateTaskAs(ctx, "owner", mwanachamataskmanager.Task{Title: "owner"}); err != nil {
		t.Fatalf("owner group: %v", err)
	}
	if err := mgr.DeleteTaskAs(ctx, "dev", task.ID); err != nil {
		t.Fatalf("dev deletes with Can move: %v", err)
	}
}

func TestBacklogWrites_AHiddenBacklogCannotBeWrittenToOrCreatedIn(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	task := softwareTask(t, mgr)
	_, err := mgr.CreateTaskAs(ctx, "dev", mwanachamataskmanager.Task{Title: "x", Backlog: "software", Step: "open"})
	wantForbidden(t, "create in a hidden backlog", err)
	_, err = mgr.CreateTaskAs(ctx, "dev", mwanachamataskmanager.Task{Title: "unplaced"})
	wantForbidden(t, "create unplaced lands in the first backlog, which dev cannot move in", err)
	wantForbidden(t, "delete hidden", mgr.DeleteTaskAs(ctx, "dev", task.ID))
}

func TestBacklogWrites_UpdatingIntoAnotherBacklogNeedsCanMoveThere(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	task := softwareTask(t, mgr)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	moved := task
	moved.Backlog = "backlog"
	moved.Step = "to-do"
	_, err := mgr.UpdateTaskAs(ctx, "dev", moved)
	wantForbidden(t, "update into the first backlog", err)
}

func TestBacklogWrites_RelationshipsNeedCanMoveOnTheFromIssueAndCanSeeOnTheOther(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	waiting := softwareTask(t, mgr)
	hidden := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "hidden"})
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}
	edge := mwanachamataskmanager.Relationship{Label: "depends_on", FromID: waiting.ID, ToID: hidden.ID}
	_, err := mgr.CreateRelationshipAs(ctx, "dev", edge)
	wantForbidden(t, "edge from a Can see issue", err)

	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	_, err = mgr.CreateRelationshipAs(ctx, "dev", edge)
	wantForbidden(t, "edge to an issue dev cannot see", err)

	if _, err := mgr.CreateRelationshipAs(ctx, "owner", edge); err != nil {
		t.Fatalf("owner: %v", err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}
	wantForbidden(t, "delete edge with Can see", mgr.DeleteRelationshipAs(ctx, "dev", waiting.ID, hidden.ID, "depends_on"))
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteRelationshipAs(ctx, "dev", waiting.ID, hidden.ID, "depends_on"); err != nil {
		t.Fatalf("dev deletes an edge of an issue it can move: %v", err)
	}
}

func TestBacklogWrites_AMissingGroupLookupSeesNothingOnceAccessIsSet(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	mgr.UseGroupExists(knownGroups(ownerGroup))
	if _, err := mgr.CreateBacklog(ctx, "Software", []string{"Open"}); err != nil {
		t.Fatal(err)
	}
	if got := visibleIDs(t, mgr, "dev"); len(got) != 2 {
		t.Fatalf("an unrestricted hub shows everything: %v", got)
	}
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	if got := visibleIDs(t, mgr, "dev"); len(got) != 0 {
		t.Fatalf("a person sees %v on a restricted hub whose groups cannot be looked up", got)
	}
	if got := visibleIDs(t, mgr, ""); len(got) != 2 {
		t.Fatalf("a call with no person is not narrowed: %v", got)
	}
	_, err := mgr.CreateTaskAs(ctx, "dev", mwanachamataskmanager.Task{Title: "x"})
	wantForbidden(t, "write with no group lookup", err)
}

func TestBacklogOwnerGroup_RefusesEmptyAndUnknownGroups(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	mgr.UseGroups(groupsOfPeople())
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("without a group lookup the group cannot be checked: %v", err)
	}
	mgr.UseGroupExists(knownGroups(ownerGroup))
	if err := mgr.SetBacklogOwnerGroup(ctx, " "); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("empty: %v", err)
	}
	if err := mgr.SetBacklogOwnerGroup(ctx, "g-nowhere"); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("unknown: %v", err)
	}
	if got := visibleIDs(t, mgr, "dev"); len(got) != 1 {
		t.Fatalf("a refused owner group must leave the hub open: %v", got)
	}
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
}

func TestBacklogOwnerGroup_TheFirstGrantNeedsAnOwnerGroup(t *testing.T) {
	ctx := context.Background()
	mgr := managerWithTwoBacklogs(t)
	mgr.UseGroupExists(knownGroups(ownerGroup, devGroup))
	_, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see")
	if !errors.Is(err, mwanachamataskmanager.ErrOwnerGroupNotSet) {
		t.Fatalf("got %v, want ErrOwnerGroupNotSet", err)
	}
	if got := visibleIDs(t, mgr, "none"); len(got) != 2 {
		t.Fatalf("a refused grant must leave the hub open: %v", got)
	}
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}
	if got := visibleIDs(t, mgr, "owner"); len(got) != 2 {
		t.Fatalf("managers keep seeing every backlog: %v", got)
	}
}

func TestBacklogOwnerGroup_ANewBacklogIsSeenByTheOwnerGroupAndNobodyElse(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateBacklog(ctx, "Design", []string{"Open"}); err != nil {
		t.Fatal(err)
	}
	if got := visibleIDs(t, mgr, "owner"); !equalIDs(got, []string{"backlog", "software", "design"}) {
		t.Fatalf("owner sees %v", got)
	}
	if got := visibleIDs(t, mgr, "dev"); !equalIDs(got, []string{"software"}) {
		t.Fatalf("dev sees %v", got)
	}
}

func TestBacklogAccess_AnIssueWhoseBacklogNoLongerExistsStaysVisibleInTheFirstBacklog(t *testing.T) {
	ctx := context.Background()
	mgr, db, tables := newTestManagerWithDB(t, nil)
	table := tables.Tasks
	mgr.UseGroups(groupsOfPeople())
	mgr.UseGroupExists(knownGroups(ownerGroup, devGroup))
	if _, err := mgr.CreateBacklog(ctx, "Software", []string{"Open"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	stray := createFilterTask(t, mgr, mwanachamataskmanager.Task{Title: "stray"})
	if err := db.Table(table).Where("id = ?", stray.ID).Update("backlog", "ghost").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "backlog", devGroup, "see"); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{RequesterID: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range got {
		if task.ID == stray.ID {
			return
		}
	}
	t.Fatalf("dev sees the first backlog, where the stray issue now reads, but not the issue: %+v", got)
}

func TestBacklogAccess_GivingToDifferentGroupsAtOnceLosesNoGrant(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	workSpec, _ := specForInstance(t, "spec/examples/work.taskmanager.json", "test")
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatal(err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
	if err != nil {
		t.Fatal(err)
	}
	groups := make([]string, 8)
	for i := range groups {
		groups[i] = fmt.Sprintf("g-extra-%d", i)
	}
	mgr.UseGroups(groupsOfPeople())
	mgr.UseGroupExists(knownGroups(append(groups, ownerGroup)...))
	if _, err := mgr.CreateBacklog(ctx, "Software", []string{"Open"}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, g := range groups {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := mgr.GiveBacklogAccess(ctx, "software", g, "see"); err != nil {
				t.Errorf("give %s: %v", g, err)
			}
		}()
	}
	wg.Wait()
	got, err := mgr.ListBacklogAccess(ctx, "software")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != len(groups) {
		t.Fatalf("%d grants survived out of %d: %+v", len(got.Groups), len(groups), got.Groups)
	}
}

func TestBacklogAccess_AGrantToAGroupThatNoLongerExistsIsListedAsGone(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}
	mgr.UseGroupExists(knownGroups(ownerGroup))
	got, err := mgr.ListBacklogAccess(ctx, "software")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 1 || !got.Groups[0].Gone {
		t.Fatalf("got %+v, want the dev grant marked gone", got.Groups)
	}
}
