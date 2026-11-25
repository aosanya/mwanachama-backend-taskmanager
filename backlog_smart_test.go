package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func rulesOf(match string, rules ...models.BacklogRule) *models.BacklogRules {
	return &models.BacklogRules{Match: match, Rules: rules}
}

func titleHas(word string) models.BacklogRule {
	return models.BacklogRule{Field: "title", Operator: "contains", Value: word}
}

func smartManager(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr := withReach(managerWithTwoBacklogs(t))
	for _, title := range []string{"Fix the import route", "Write the guide", "Import the catalog"} {
		mustCreate(t, mgr, mwanachamataskmanager.Task{Title: title})
	}
	return mgr
}

func TestSmartBacklog_CreateListsItWithItsRulesAndLiveCount(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	made, err := mgr.CreateBacklogWithRules(ctx, "Imports", nil, rulesOf("all", titleHas("Import")))
	if err != nil {
		t.Fatal(err)
	}
	if made.Kind != models.BacklogKindSmart || made.Rules == nil || len(made.Steps) != 0 || made.Issues != 2 {
		t.Fatalf("created %+v", made)
	}
	list, err := mgr.ListBacklogsFor(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	last := list.Items[len(list.Items)-1]
	if last.ID != made.ID || last.Issues != 2 || last.Rules.Match != "all" {
		t.Fatalf("listed %+v", last)
	}
}

func TestSmartBacklog_TheSameRulesCannotBeSavedTwiceAndTheRefusalNamesTheExistingOne(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	first, err := mgr.CreateBacklogWithRules(ctx, "Imports", nil, rulesOf("all", titleHas("Import")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.CreateBacklogWithRules(ctx, "Other name", nil, rulesOf("all", titleHas("  import ")))
	if !errors.Is(err, mwanachamataskmanager.ErrDuplicateRules) || !strings.Contains(err.Error(), `"existing_id":"`+first.ID+`"`) || !strings.Contains(err.Error(), `"existing_name":"Imports"`) {
		t.Fatalf("got %v", err)
	}
	if _, err := mgr.CreateBacklogWithRules(ctx, "Guides", nil, rulesOf("all", titleHas("guide"))); err != nil {
		t.Fatal(err)
	}
}

func TestSmartBacklog_RefusesBadRulesAndStepsOnASmartBacklog(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	bad := []*models.BacklogRules{
		rulesOf("all"),
		rulesOf("all", models.BacklogRule{Field: "colour", Operator: "is", Value: "red"}),
		rulesOf("all", models.BacklogRule{Field: "title", Operator: "is", Value: "x"}),
		rulesOf("sometimes", titleHas("x")),
	}
	for _, rules := range bad {
		if _, err := mgr.CreateBacklogWithRules(ctx, "Bad", nil, rules); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
			t.Fatalf("%+v: got %v", rules, err)
		}
	}
	if _, err := mgr.CreateBacklogWithRules(ctx, "Both", []string{"Open"}, rulesOf("all", titleHas("x"))); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("steps with rules: got %v", err)
	}
}

func TestSmartBacklog_SetRulesChangesWhatIsInItAndRefusesADuplicate(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	imports, _ := mgr.CreateBacklogWithRules(ctx, "Imports", nil, rulesOf("all", titleHas("Import")))
	guides, _ := mgr.CreateBacklogWithRules(ctx, "Guides", nil, rulesOf("all", titleHas("guide")))
	got, err := mgr.SetBacklogRules(ctx, guides.ID, rulesOf("any", titleHas("guide"), titleHas("route")))
	if err != nil || got.Issues != 2 {
		t.Fatalf("set: %+v %v", got, err)
	}
	if _, err := mgr.SetBacklogRules(ctx, guides.ID, rulesOf("all", titleHas("import"))); !errors.Is(err, mwanachamataskmanager.ErrDuplicateRules) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := mgr.SetBacklogRules(ctx, "software", rulesOf("all", titleHas("x"))); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("a backlog with steps takes no rules: %v", err)
	}
	if _, err := mgr.SetBacklogRules(ctx, imports.ID, rulesOf("all", titleHas("import"))); err != nil {
		t.Fatalf("saving its own rules again is fine: %v", err)
	}
}

func TestSmartBacklog_CountBeforeSavingAndListingThroughIt(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	count, err := mgr.CountBacklogRules(ctx, "", rulesOf("all", titleHas("the")))
	if err != nil || count.Count != 3 {
		t.Fatalf("count %+v %v", count, err)
	}
	made, _ := mgr.CreateBacklogWithRules(ctx, "Imports", nil, rulesOf("all", titleHas("import")))
	tasks, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Backlog: made.ID})
	if err != nil || len(tasks) != 2 {
		t.Fatalf("through the smart backlog: %d %v", len(tasks), err)
	}
	if _, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{Backlog: made.ID, Step: "Open"}); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklogStep) {
		t.Fatalf("a step on a smart backlog: %v", err)
	}
}

func TestSmartBacklog_IsNoMoveTargetTakesOnlySeeAndEditAndRenamesAndDeletes(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	made, _ := mgr.CreateBacklogWithRules(ctx, "Imports", nil, rulesOf("all", titleHas("import")))
	tasks, _ := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if _, err := mgr.MoveTaskToStep(ctx, tasks[0].ID, made.ID, "open"); err == nil {
		t.Fatal("a smart backlog took an issue")
	}
	if _, err := giveAccess(mgr, ctx, made.ID, devGroup, "move"); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("move level: %v", err)
	}
	for _, level := range []string{"edit"} {
		if _, err := giveAccess(mgr, ctx, made.ID, devGroup, level); err != nil {
			t.Fatalf("%s: %v", level, err)
		}
	}
	if got, err := mgr.RenameBacklog(ctx, made.ID, "Imports now"); err != nil || got.Name != "Imports now" {
		t.Fatalf("rename: %+v %v", got, err)
	}
	if err := mgr.DeleteBacklog(ctx, made.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestSmartBacklog_TwoCallersSeeDifferentCounts(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	if err := mgr.SetBacklogOwnerGroup(ctx, ownerGroup); err != nil {
		t.Fatal(err)
	}
	made, err := mgr.CreateBacklogWithRules(ctx, "Everything", nil, rulesOf("all", titleHas("the")))
	if err != nil {
		t.Fatal(err)
	}
	count := func(person string) int {
		list, err := mgr.ListBacklogsFor(ctx, person)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range list.Items {
			if item.ID == made.ID {
				return item.Issues
			}
		}
		return -1
	}
	if owner, dev := count("owner"), count("dev"); owner != 3 || dev != 0 {
		t.Fatalf("owner %d, dev %d: the dev group sees the smart backlog but none of the issues in other backlogs", owner, dev)
	}
}

func TestSmartBacklog_SetRulesNeedsRules(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	guides, err := mgr.CreateBacklogWithRules(ctx, "Guides", nil, rulesOf("all", titleHas("guide")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SetBacklogRules(ctx, guides.ID, nil); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("no rules: %v", err)
	}
	if _, err := mgr.CountBacklogRules(ctx, "", nil); !errors.Is(err, mwanachamataskmanager.ErrInvalidBacklog) {
		t.Fatalf("counting no rules: %v", err)
	}
}

func TestSmartBacklog_AWaitsOnRuleSeparatesTheIssuesThatWaitFromTheOnesThatDoNot(t *testing.T) {
	mgr := smartManager(t)
	ctx := context.Background()
	waiting := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Waits on the route"})
	blocker := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "The route"})
	edge := mwanachamataskmanager.Relationship{Label: "depends_on", FromID: waiting.ID, ToID: blocker.ID}
	if _, err := mgr.CreateRelationship(ctx, edge); err != nil {
		t.Fatal(err)
	}
	waits, err := mgr.CreateBacklogWithRules(ctx, "Waiting", nil, rulesOf("all", models.BacklogRule{Field: "waits_on", Operator: "has_something"}))
	if err != nil {
		t.Fatal(err)
	}
	free, err := mgr.CreateBacklogWithRules(ctx, "Free", nil, rulesOf("all", models.BacklogRule{Field: "waits_on", Operator: "has_nothing"}))
	if err != nil {
		t.Fatal(err)
	}
	if waits.Issues != 1 || free.Issues != 4 {
		t.Fatalf("waiting %d, free %d, want 1 and 4", waits.Issues, free.Issues)
	}
}
