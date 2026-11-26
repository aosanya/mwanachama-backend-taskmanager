package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestRules_ARuleGetsTheNextRuleCodeAndStartsProposed(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	first, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "No comments in code", Body: "A name carries it."})
	if err != nil {
		t.Fatal(err)
	}
	second, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "Font sizes are in rem"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Code != "R-1" || second.Code != "R-2" {
		t.Fatalf("codes %q and %q, want R-1 and R-2", first.Code, second.Code)
	}
	if first.Status != "proposed" || first.ID == "" || first.CreatedAt == "" {
		t.Fatalf("got %+v", first)
	}
}

func TestRules_ADecisionHasItsOwnCodeSeries(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	mustCreateRule(t, mgr, "rule", "A rule")
	decision := mustCreateRule(t, mgr, "decision", "A choice made once")
	if decision.Code != "D-1" {
		t.Fatalf("code %q, want D-1", decision.Code)
	}
	again, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "decision", Title: "Another"})
	if err != nil || again.Code != "D-2" {
		t.Fatalf("code %q, err %v, want D-2", again.Code, err)
	}
}

func TestRules_ReadByCodeReturnsWhatWasWritten(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	created, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{
		Kind: "rule", Title: "Work on main", Body: "Commit on main.", Tags: []string{"git"}, DecidedBy: "Owner",
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := mgr.GetRuleByCode(ctx, "owner", "R-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != created.ID || got.Title != "Work on main" || got.Body != "Commit on main." || got.DecidedBy != "Owner" || len(got.Tags) != 1 || got.Tags[0] != "git" {
		t.Fatalf("got %+v", got)
	}
}

func TestRules_AnUnknownCodeIsNotFound(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.GetRuleByCode(context.Background(), "owner", "R-99"); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("err %v, want ErrRuleNotFound", err)
	}
}

func TestRules_ARecordNeedsAKindAndATitle(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	for _, r := range []mwanachamataskmanager.RuleRecord{{Kind: "rule"}, {Title: "x"}, {Kind: "law", Title: "x"}} {
		if _, err := mgr.CreateRule(ctx, "owner", r); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%+v: err %v, want ErrInvalidRule", r, err)
		}
	}
}

func mustCreateRule(t *testing.T, mgr mwanachamataskmanager.TaskManager, kind, title string) mwanachamataskmanager.RuleRecord {
	t.Helper()
	r, err := mgr.CreateRule(context.Background(), "owner", mwanachamataskmanager.RuleRecord{Kind: kind, Title: title, TargetRole: roleOfKind(kind)})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRules_UpdateChangesTheWordsAndKeepsCodeKindAndStatus(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	created := mustCreateRule(t, mgr, "rule", "Old title")
	title, body, tags := "New title", "Because.", []string{"css", "app"}

	got, err := mgr.UpdateRule(ctx, "owner", created.Code, mwanachamataskmanager.RulePatch{Title: &title, Body: &body, Tags: &tags})
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != created.Code || got.Kind != "rule" || got.Status != "proposed" || got.Title != "New title" || got.Body != "Because." || len(got.Tags) != 2 {
		t.Fatalf("got %+v", got)
	}
	read, err := mgr.GetRuleByCode(ctx, "owner", created.Code)
	if err != nil || read.Title != "New title" || read.UpdatedAt == "" {
		t.Fatalf("read %+v, err %v", read, err)
	}
}

func TestRules_UpdateRefusesAnEmptyTitleAndAnUnknownCode(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	created := mustCreateRule(t, mgr, "rule", "A")
	empty := "  "
	if _, err := mgr.UpdateRule(ctx, "owner", created.Code, mwanachamataskmanager.RulePatch{Title: &empty}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
	title := "x"
	if _, err := mgr.UpdateRule(ctx, "owner", "R-99", mwanachamataskmanager.RulePatch{Title: &title}); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("err %v, want ErrRuleNotFound", err)
	}
}

func TestRules_ListHidesSupersededAndRetiredUnlessAsked(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	proposed := mustCreateRule(t, mgr, "rule", "proposed")
	inForce := mustCreateRule(t, mgr, "rule", "in force")
	retired := mustCreateRule(t, mgr, "rule", "retired")
	mustSetStatus(t, mgr, inForce.Code, "in_force")
	mustSetStatus(t, mgr, retired.Code, "in_force")
	mustSetStatus(t, mgr, retired.Code, "retired")

	got, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "rule"})
	if err != nil {
		t.Fatal(err)
	}
	if codes(got) != proposed.Code+","+inForce.Code {
		t.Fatalf("default list %q", codes(got))
	}
	all, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "rule", Statuses: []string{"proposed", "in_force", "retired", "superseded"}})
	if err != nil || len(all) != 3 {
		t.Fatalf("all: %d, err %v", len(all), err)
	}
}

func TestRules_ListFiltersByKindAndByEveryTagGiven(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	a, _ := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "a", Tags: []string{"css", "app"}})
	mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "b", Tags: []string{"css"}})
	mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "decision", Title: "c", Tags: []string{"css", "app"}})

	got, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "rule", Tags: []string{"css", "app"}})
	if err != nil || codes(got) != a.Code {
		t.Fatalf("got %q, err %v, want %s", codes(got), err, a.Code)
	}
	both, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Tags: []string{"css"}})
	if err != nil || len(both) != 3 {
		t.Fatalf("no kind: %d, err %v, want 3", len(both), err)
	}
}

func TestRules_ListRefusesAStatusOfTheOtherKind(t *testing.T) {
	_, err := newTestManager(t).ListRules(context.Background(), "owner", mwanachamataskmanager.RuleFilter{Kind: "decision", Statuses: []string{"retired"}})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}

func TestRules_StatusIsCheckedPerKind(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "r")
	decision := mustCreateRule(t, mgr, "decision", "d")

	if got, err := mgr.SetRuleStatus(ctx, "owner", rule.Code, "in_force"); err != nil || got.Status != "in_force" {
		t.Fatalf("rule in_force: %+v, %v", got, err)
	}
	if got, err := mgr.SetRuleStatus(ctx, "owner", decision.Code, "decided"); err != nil || got.Status != "decided" {
		t.Fatalf("decision decided: %+v, %v", got, err)
	}
	if _, err := mgr.SetRuleStatus(ctx, "owner", decision.Code, "retired"); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("decision retired: err %v, want ErrInvalidRule", err)
	}
	if _, err := mgr.SetRuleStatus(ctx, "owner", rule.Code, "decided"); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("rule decided: err %v, want ErrInvalidRule", err)
	}
	if _, err := mgr.SetRuleStatus(ctx, "owner", rule.Code, "superseded"); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("superseded by hand: err %v, want ErrInvalidRule", err)
	}
}
