package mwanachamataskmanager_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func accessRaceManager(t *testing.T, groups int) (mwanachamataskmanager.TaskManager, []string) {
	t.Helper()
	mgr := newFileBackedManager(t)
	ids := []string{ownerGroup}
	members := make([]string, groups)
	for i := range members {
		members[i] = fmt.Sprintf("group-%d", i)
		ids = append(ids, members[i])
	}
	mgr.UseGroupExists(knownGroups(ids...))
	if err := mgr.SetBacklogOwnerGroup(context.Background(), ownerGroup); err != nil {
		t.Fatalf("SetBacklogOwnerGroup: %v", err)
	}
	return mgr, members
}

func giveAll(mgr mwanachamataskmanager.TaskManager, backlogID string, groups []string) []error {
	errs := make([]error, len(groups))
	var wg sync.WaitGroup
	for i, group := range groups {
		wg.Add(1)
		go func(i int, group string) {
			defer wg.Done()
			_, errs[i] = mgr.GiveBacklogAccess(context.Background(), backlogID, group, "see")
		}(i, group)
	}
	wg.Wait()
	return errs
}

func requireAllGranted(t *testing.T, mgr mwanachamataskmanager.TaskManager, backlogID string, groups []string) {
	t.Helper()
	access, err := mgr.ListBacklogAccess(context.Background(), backlogID)
	if err != nil {
		t.Fatalf("ListBacklogAccess: %v", err)
	}
	held := map[string]bool{}
	for _, g := range access.Groups {
		held[g.GroupID] = true
	}
	for _, group := range groups {
		if !held[group] {
			t.Fatalf("group %q lost its access to %q; held %v", group, backlogID, held)
		}
	}
}

func TestSimultaneousAccessGivesAllSucceedAndAllSurvive(t *testing.T) {
	mgr, groups := accessRaceManager(t, 8)
	created, err := mgr.CreateBacklog(context.Background(), "Design", []string{"Open", "Done"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	for i, err := range giveAll(mgr, created.ID, groups) {
		if err != nil {
			t.Fatalf("give %d: %v", i, err)
		}
	}
	requireAllGranted(t, mgr, created.ID, groups)
}

func TestSimultaneousAccessGivesSurviveADeleteThatPrunesGrants(t *testing.T) {
	mgr, groups := accessRaceManager(t, 9)
	ctx := context.Background()
	kept, err := mgr.CreateBacklog(ctx, "Kept", []string{"Open"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	doomed, err := mgr.CreateBacklog(ctx, "Doomed", []string{"Open"})
	if err != nil {
		t.Fatalf("CreateBacklog: %v", err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, doomed.ID, groups[8], "see"); err != nil {
		t.Fatalf("give on doomed: %v", err)
	}

	var wg sync.WaitGroup
	var deleteErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		deleteErr = mgr.DeleteBacklog(ctx, doomed.ID)
	}()
	errs := giveAll(mgr, kept.ID, groups[:8])
	wg.Wait()

	if deleteErr != nil {
		t.Fatalf("DeleteBacklog: %v", deleteErr)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("give %d: %v", i, err)
		}
	}
	requireAllGranted(t, mgr, kept.ID, groups[:8])
}
