package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func stepNames(summary mwanachamataskmanager.BacklogSummary) []string {
	names := make([]string, len(summary.Steps))
	for i, step := range summary.Steps {
		names[i] = step.Name
	}
	return names
}

func sameNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func softwareSummary(t *testing.T, mgr mwanachamataskmanager.TaskManager, person string) mwanachamataskmanager.BacklogSummary {
	t.Helper()
	list, err := mgr.ListBacklogsFor(context.Background(), person)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Items {
		if item.ID == "software" {
			return item
		}
	}
	t.Fatalf("%s does not see the software backlog", person)
	return mwanachamataskmanager.BacklogSummary{}
}

func TestBacklogEditWrappers_CanMoveIsRefusedOnEveryStepAndAccessChange(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}

	_, err := mgr.AddBacklogStepAs(ctx, "dev", "software", "Review", "")
	wantForbidden(t, "add a step", err)
	_, err = mgr.RenameBacklogStepAs(ctx, "dev", "software", "open", "To do")
	wantForbidden(t, "rename a step", err)
	_, err = mgr.ReorderBacklogStepsAs(ctx, "dev", "software", []string{"shipped", "open"})
	wantForbidden(t, "reorder the steps", err)
	_, err = mgr.RemoveBacklogStepAs(ctx, "dev", "software", "shipped")
	wantForbidden(t, "remove a step", err)
	wantForbidden(t, "delete the backlog", mgr.DeleteBacklogAs(ctx, "dev", "software"))
	_, err = mgr.GiveBacklogAccessAs(ctx, "dev", "software", docsGroup, "see")
	wantForbidden(t, "give access", err)
	_, err = mgr.TakeBacklogAccessAs(ctx, "dev", "software", devGroup)
	wantForbidden(t, "take access", err)

	if got := stepNames(softwareSummary(t, mgr, "dev")); !sameNames(got, []string{"Open", "Shipped"}) {
		t.Fatalf("a refused edit changed the steps: %v", got)
	}
	access, err := mgr.ListBacklogAccess(ctx, "software")
	if err != nil {
		t.Fatal(err)
	}
	if len(access.Groups) != 1 || access.Groups[0].GroupID != devGroup || string(access.Groups[0].Level) != "move" {
		t.Fatalf("a refused edit changed the access: %+v", access.Groups)
	}
}

func TestBacklogEditWrappers_CanEditChangesStepsAndAccessOfThatBacklogOnly(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	docs, err := mgr.CreateBacklog(ctx, "Docs", []string{"Draft"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "edit"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, docs.ID, devGroup, "see"); err != nil {
		t.Fatal(err)
	}

	added, err := mgr.AddBacklogStepAs(ctx, "dev", "software", "Review", "open")
	if err != nil {
		t.Fatalf("add a step with Can edit: %v", err)
	}
	if got := stepNames(added); !sameNames(got, []string{"Open", "Review", "Shipped"}) {
		t.Fatalf("steps after adding Review after Open: %v", got)
	}
	renamed, err := mgr.RenameBacklogStepAs(ctx, "dev", "software", "open", "To do")
	if err != nil {
		t.Fatalf("rename a step with Can edit: %v", err)
	}
	if got := stepNames(renamed); !sameNames(got, []string{"To do", "Review", "Shipped"}) {
		t.Fatalf("steps after the rename: %v", got)
	}
	ids := make([]string, len(renamed.Steps))
	for i, step := range renamed.Steps {
		ids[i] = step.ID
	}
	reversed, err := mgr.ReorderBacklogStepsAs(ctx, "dev", "software", []string{ids[2], ids[1], ids[0]})
	if err != nil {
		t.Fatalf("reorder the steps with Can edit: %v", err)
	}
	if got := stepNames(reversed); !sameNames(got, []string{"Shipped", "Review", "To do"}) {
		t.Fatalf("steps after reversing: %v", got)
	}
	trimmed, err := mgr.RemoveBacklogStepAs(ctx, "dev", "software", ids[1])
	if err != nil {
		t.Fatalf("remove a step with Can edit: %v", err)
	}
	if got := stepNames(trimmed); !sameNames(got, []string{"Shipped", "To do"}) {
		t.Fatalf("steps after removing Review: %v", got)
	}

	given, err := mgr.GiveBacklogAccessAs(ctx, "dev", "software", docsGroup, "see")
	if err != nil || len(given.Groups) != 2 {
		t.Fatalf("give access with Can edit: %+v %v", given, err)
	}
	taken, err := mgr.TakeBacklogAccessAs(ctx, "dev", "software", docsGroup)
	if err != nil || len(taken.Groups) != 1 || taken.Groups[0].GroupID != devGroup {
		t.Fatalf("take access with Can edit: %+v %v", taken, err)
	}

	_, err = mgr.AddBacklogStepAs(ctx, "dev", docs.ID, "Review", "")
	wantForbidden(t, "Can see on Docs does not edit it", err)
	wantForbidden(t, "delete Docs", mgr.DeleteBacklogAs(ctx, "dev", docs.ID))
}

func TestBacklogEditWrappers_CanEditDeletesAnEmptyBacklogAndAHoldingOneRefuses(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "edit"); err != nil {
		t.Fatal(err)
	}
	softwareTask(t, mgr)

	err := mgr.DeleteBacklogAs(ctx, "dev", "software")
	if err == nil {
		t.Fatal("a backlog that holds an issue was deleted")
	}
	if errors.Is(err, mwanachamataskmanager.ErrBacklogForbidden) {
		t.Fatalf("a person with Can edit was refused on access: %v", err)
	}
	if got := stepNames(softwareSummary(t, mgr, "dev")); len(got) != 2 {
		t.Fatalf("a refused delete changed the backlog: %v", got)
	}

	empty, err := mgr.CreateBacklog(ctx, "Scratch", []string{"Open"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.GiveBacklogAccess(ctx, empty.ID, devGroup, "edit"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteBacklogAs(ctx, "dev", empty.ID); err != nil {
		t.Fatalf("delete an empty backlog with Can edit: %v", err)
	}
	for _, id := range visibleIDs(t, mgr, "owner") {
		if id == empty.ID {
			t.Fatalf("the deleted backlog is still listed: %v", visibleIDs(t, mgr, "owner"))
		}
	}
}

func TestBacklogEditWrappers_ReplacingEveryBacklogNeedsCanEditOnAllOfThem(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "edit"); err != nil {
		t.Fatal(err)
	}
	keep := []mwanachamataskmanager.Backlog{{ID: "software", Name: "Software", Steps: []mwanachamataskmanager.BacklogStep{{ID: "open", Name: "Open"}, {ID: "shipped", Name: "Shipped"}}}}

	_, err := mgr.ReplaceBacklogsAs(ctx, "dev", keep)
	wantForbidden(t, "Can edit on one of two backlogs", err)
	if len(visibleIDs(t, mgr, "owner")) != 2 {
		t.Fatalf("a refused replace changed the backlogs: %v", visibleIDs(t, mgr, "owner"))
	}

	list, err := mgr.ReplaceBacklogsAs(ctx, "owner", keep)
	if err != nil {
		t.Fatalf("the Product owner group replaces the backlogs: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != "software" {
		t.Fatalf("backlogs after the replace: %+v", list.Items)
	}
}
