package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func levelsOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, person string) map[string]models.AccessLevel {
	t.Helper()
	list, err := mgr.ListBacklogsFor(context.Background(), person)
	if err != nil {
		t.Fatal(err)
	}
	levels := map[string]models.AccessLevel{}
	for _, item := range list.Items {
		levels[item.ID] = item.Level
	}
	return levels
}

func TestBacklogList_EachBacklogCarriesTheCallersLevel(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	ctx := context.Background()
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	if _, err := giveAccess(mgr, ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	if _, err := giveSee(mgr, ctx, "software", docsGroup); err != nil {
		t.Fatal(err)
	}
	if got := levelsOf(t, mgr, "owner"); got["software"] != models.AccessEdit || got["backlog"] != models.AccessEdit {
		t.Fatalf("owner levels %v", got)
	}
	if got := levelsOf(t, mgr, "dev"); len(got) != 1 || got["software"] != models.AccessMove {
		t.Fatalf("dev levels %v", got)
	}
	if got := levelsOf(t, mgr, "both"); len(got) != 1 || got["software"] != models.AccessMove {
		t.Fatalf("a person in two groups gets the higher level, got %v", got)
	}
}

func TestBacklogList_AnOpenHubShowsEditEverywhere(t *testing.T) {
	mgr := managerWithTwoBacklogs(t)
	for _, level := range levelsOf(t, mgr, "none") {
		if level != models.AccessEdit {
			t.Fatalf("an open hub gave %q", level)
		}
	}
}
