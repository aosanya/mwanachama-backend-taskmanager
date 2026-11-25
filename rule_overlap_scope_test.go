package mwanachamataskmanager_test

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestRuleOverlap_AnIssueInABacklogTheCallerCannotSeeStaysOutOfSharedIssues(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	seen := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in software", Tags: []string{"css", "app"}, Backlog: "software", Step: "open"})
	hidden := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in the first backlog", Tags: []string{"css", "app"}})
	ruleInForceWhen(t, mgr, "Use rem", tagWhen("css"))
	ruleInForceWhen(t, mgr, "Keep it small", tagWhen("app"))
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}

	devList, err := mgr.ListRuleOverlaps(ctx, "dev")
	if err != nil {
		t.Fatal(err)
	}
	asDev := devList.Items
	ownerList, err := mgr.ListRuleOverlaps(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	asOwner := ownerList.Items

	if len(asDev) != 1 || len(asDev[0].SharedIssues) != 1 || asDev[0].SharedIssues[0] != seen.Code {
		t.Fatalf("dev saw %+v, want only %s", asDev, seen.Code)
	}
	if len(asOwner) != 1 || len(asOwner[0].SharedIssues) != 2 {
		t.Fatalf("owner saw %+v, want %s and %s", asOwner, seen.Code, hidden.Code)
	}
}

func TestRuleOverlap_OnlyOpenIssuesMakeTwoSetsOverlap(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	finished := issueTagged(t, mgr, "Old css work", "css", "app")
	dropped := issueTagged(t, mgr, "Dropped css work", "css", "app")
	ruleInForceWhen(t, mgr, "Use rem", tagWhen("css"))
	ruleInForceWhen(t, mgr, "Keep it small", tagWhen("app"))
	if _, err := mgr.MoveTaskToStep(ctx, finished.ID, "backlog", "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.MoveTaskToStep(ctx, dropped.ID, "backlog", "cancelled"); err != nil {
		t.Fatal(err)
	}

	if got := overlaps(t, mgr); len(got) != 0 {
		t.Fatalf("closed issues made an overlap: %+v", got)
	}
	open := issueTagged(t, mgr, "Open css work", "css", "app")
	got := overlaps(t, mgr)
	if len(got) != 1 || len(got[0].SharedIssues) != 1 || got[0].SharedIssues[0] != open.Code {
		t.Fatalf("overlaps %+v, want only %s", got, open.Code)
	}
}

func queryCounter(t *testing.T) (mwanachamataskmanager.TaskManager, func() int) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	work := specForInstance(t, "spec/examples/agency.taskmanager.json", "test")
	if err := spec.Migrate(db, work); err != nil {
		t.Fatal(err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, work, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	tick := func(*gorm.DB) { count++ }
	if err := db.Callback().Query().Register("count_queries", tick); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Row().Register("count_rows", tick); err != nil {
		t.Fatal(err)
	}
	return mgr, func() int { c := count; count = 0; return c }
}

func TestRuleOverlap_QueriesDoNotGrowWithTheNumberOfIssues(t *testing.T) {
	mgr, queries := queryCounter(t)
	ruleInForceWhen(t, mgr, "Use rem", tagWhen("css"))
	ruleInForceWhen(t, mgr, "Keep it small", tagWhen("app"))
	issueTagged(t, mgr, "tagged", "css", "app")
	issueTagged(t, mgr, "plain")
	overlaps(t, mgr)
	queries()
	overlaps(t, mgr)
	few := queries()
	for i := 0; i < 15; i++ {
		issueTagged(t, mgr, "more", "css", "app")
		issueTagged(t, mgr, "plain")
	}
	queries()

	overlaps(t, mgr)

	if many := queries(); many != few {
		t.Fatalf("%d queries with 1 issue, %d with 16", few, many)
	}
}

func TestSmartBacklog_QueriesDoNotGrowWithTheNumberOfIssuesAndTagRulesStillMatch(t *testing.T) {
	ctx := context.Background()
	mgr, queries := queryCounter(t)
	tagged := issueTagged(t, mgr, "tagged", "css")
	issueTagged(t, mgr, "plain")
	made, err := mgr.CreateBacklogWithRules(ctx, "Css", nil, rulesOf("all", models.BacklogRule{Field: "tag", Operator: "has", Value: "css"}))
	if err != nil {
		t.Fatal(err)
	}
	queries()
	listed := func() int {
		list, err := mgr.ListBacklogsFor(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range list.Items {
			if item.ID == made.ID {
				return item.Issues
			}
		}
		t.Fatal("smart backlog not listed")
		return -1
	}
	if listed() != 1 {
		t.Fatal("the tagged issue is not in the smart backlog")
	}
	few := queries()
	for i := 0; i < 15; i++ {
		issueTagged(t, mgr, "plain more")
	}
	queries()

	if listed() != 1 {
		t.Fatal("untagged issues entered the smart backlog")
	}
	if many := queries(); many != few {
		t.Fatalf("%d queries with 2 issues, %d with 17", few, many)
	}
	tags := []string{}
	if _, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: tagged.ID, Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	if listed() != 0 {
		t.Fatal("an issue whose tags were cleared stayed in the smart backlog")
	}
}
