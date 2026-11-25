package mwanachamataskmanager_test

import (
	"context"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestRules_CreateCleansTagsKeepingTheFirstSpelling(t *testing.T) {
	mgr := newTestManager(t)
	got, err := mgr.CreateRule(context.Background(), "owner", mwanachamataskmanager.RuleRecord{
		Kind: "rule", Title: "a", Tags: []string{" CSS ", "", "css", "App", "  ", "app"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Tags, ",") != "CSS,App" {
		t.Fatalf("tags %q, want CSS,App", got.Tags)
	}
}

func TestRules_UpdateCleansTagsKeepingTheFirstSpelling(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	created := mustCreateRule(t, mgr, "rule", "a")
	tags := []string{"Go", " go ", "", "Web"}
	got, err := mgr.UpdateRule(ctx, "owner", created.Code, mwanachamataskmanager.RulePatch{Tags: &tags})
	if err != nil {
		t.Fatal(err)
	}
	read, err := mgr.GetRuleByCode(ctx, "owner", created.Code)
	if err != nil || strings.Join(got.Tags, ",") != "Go,Web" || strings.Join(read.Tags, ",") != "Go,Web" {
		t.Fatalf("returned %q, stored %q, err %v, want Go,Web", got.Tags, read.Tags, err)
	}
}

func TestRules_SupersedeCleansTheReplacementsTags(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := mustCreateRule(t, mgr, "rule", "a")
	mustSetStatus(t, mgr, old.Code, "in_force")
	next, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "b", Tags: []string{"Code", "code", " "}})
	if err != nil || strings.Join(next.Tags, ",") != "Code" {
		t.Fatalf("tags %q, err %v, want Code", next.Tags, err)
	}
}

func TestRules_ATagOnARuleIsRegisteredInTheSharedTagSet(t *testing.T) {
	ctx := context.Background()
	mgr, db, tables := newTagTestManager(t)
	if _, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "a", Tags: []string{"fromrule"}}); err != nil {
		t.Fatal(err)
	}
	tags := []string{"fromupdate"}
	if _, err := mgr.UpdateRule(ctx, "owner", "R-1", mwanachamataskmanager.RulePatch{Tags: &tags}); err != nil {
		t.Fatal(err)
	}
	if row := getTagRowByName(t, db, tables, "fromrule"); row.Code == "" {
		t.Fatalf("tag row %+v has no code", row)
	}
	getTagRowByName(t, db, tables, "fromupdate")
}
