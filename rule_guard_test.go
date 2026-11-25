package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func mustReadRule(t *testing.T, mgr mwanachamataskmanager.TaskManager, code string) mwanachamataskmanager.RuleRecord {
	t.Helper()
	r, err := mgr.GetRuleByCode(context.Background(), "owner", code)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func inForceRule(t *testing.T, mgr mwanachamataskmanager.TaskManager, title string) mwanachamataskmanager.RuleRecord {
	t.Helper()
	r := mustCreateRule(t, mgr, "rule", title)
	mustSetStatus(t, mgr, r.Code, "in_force")
	return mustReadRule(t, mgr, r.Code)
}

func TestRuleSupersede_ARuleReplacementCarriesNoPriorityAndAQuestionStyleAnswerIsCleared(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := inForceRule(t, mgr, "old")
	if _, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "new", Priority: "P1"}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("priority on a replacement rule: err %v, want ErrInvalidRule", err)
	}
	if mustReadRule(t, mgr, old.Code).Status != "in_force" {
		t.Fatal("a refused replacement must leave the old record in force")
	}
	next, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "new", AnsweredBy: "D-9"})
	if err != nil || next.AnsweredBy != "" || next.Priority != "" {
		t.Fatalf("got %+v, err %v", next, err)
	}
	if stored := mustReadRule(t, mgr, next.Code); stored.AnsweredBy != "" || stored.Priority != "" {
		t.Fatalf("stored %+v", stored)
	}
}

func TestRuleSupersede_ADecisionReplacementRefusesAPriorityToo(t *testing.T) {
	mgr := newTestManager(t)
	d := mustCreateRule(t, mgr, "decision", "old")
	mustSetStatus(t, mgr, d.Code, "decided")
	_, err := mgr.SupersedeRule(context.Background(), "owner", d.Code, mwanachamataskmanager.RuleRecord{Title: "new", Priority: "P2"})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}

func TestRuleGuard_AStaleSupersedeMakesNoSecondReplacement(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := inForceRule(t, mgr, "old")
	stale := mustReadRule(t, mgr, old.Code)
	if _, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "first"}); err != nil {
		t.Fatal(err)
	}
	_, err := mwanachamataskmanager.SupersedeRuleFromRead(mgr, ctx, stale, mwanachamataskmanager.RuleRecord{Title: "second"})
	if !errors.Is(err, mwanachamataskmanager.ErrRuleChanged) {
		t.Fatalf("err %v, want ErrRuleChanged", err)
	}
	all, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "rule", Statuses: []string{"proposed", "in_force", "retired", "superseded"}})
	if err != nil || len(all) != 2 {
		t.Fatalf("%d records %q, err %v, want the old one and one replacement", len(all), codes(all), err)
	}
	if kept := mustReadRule(t, mgr, old.Code); kept.SupersededBy != "R-2" {
		t.Fatalf("old %+v", kept)
	}
}

func TestRuleGuard_AStaleUpdateCannotReviveASupersededRule(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := inForceRule(t, mgr, "old")
	stale := mustReadRule(t, mgr, old.Code)
	if _, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "new"}); err != nil {
		t.Fatal(err)
	}
	title := "edited late"
	if _, err := mwanachamataskmanager.UpdateRuleFromRead(mgr, ctx, stale, mwanachamataskmanager.RulePatch{Title: &title}); !errors.Is(err, mwanachamataskmanager.ErrRuleChanged) {
		t.Fatalf("err %v, want ErrRuleChanged", err)
	}
	if kept := mustReadRule(t, mgr, old.Code); kept.Status != "superseded" || kept.SupersededBy != "R-2" || kept.Title != "old" {
		t.Fatalf("old %+v", kept)
	}
}

func TestRuleGuard_AStaleStatusChangeCannotReviveASupersededRule(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := inForceRule(t, mgr, "old")
	stale := mustReadRule(t, mgr, old.Code)
	if _, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "new"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mwanachamataskmanager.SetRuleStatusFromRead(mgr, ctx, stale, "retired"); !errors.Is(err, mwanachamataskmanager.ErrRuleChanged) {
		t.Fatalf("err %v, want ErrRuleChanged", err)
	}
	if kept := mustReadRule(t, mgr, old.Code); kept.Status != "superseded" {
		t.Fatalf("old %+v", kept)
	}
}

func TestRuleGuard_AStaleAnswerCannotAnswerAQuestionDroppedMeanwhile(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P1")
	d := mustCreateRule(t, mgr, "decision", "d")
	stale := mustReadRule(t, mgr, q.Code)
	mustSetStatus(t, mgr, q.Code, "dropped")
	if _, err := mwanachamataskmanager.AnswerQuestionFromRead(mgr, ctx, stale, mustReadRule(t, mgr, d.Code)); !errors.Is(err, mwanachamataskmanager.ErrRuleChanged) {
		t.Fatalf("err %v, want ErrRuleChanged", err)
	}
	if kept := mustReadRule(t, mgr, q.Code); kept.Status != "dropped" || kept.AnsweredBy != "" {
		t.Fatalf("question %+v", kept)
	}
}

func TestRuleUpdate_ASupersededOrRetiredRecordKeepsItsWords(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := inForceRule(t, mgr, "old")
	if _, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "new"}); err != nil {
		t.Fatal(err)
	}
	retired := inForceRule(t, mgr, "retiring")
	mustSetStatus(t, mgr, retired.Code, "retired")
	title := "edited"
	for _, code := range []string{old.Code, retired.Code} {
		if _, err := mgr.UpdateRule(ctx, "owner", code, mwanachamataskmanager.RulePatch{Title: &title}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s: err %v, want ErrInvalidRule", code, err)
		}
		if mustReadRule(t, mgr, code).Title == "edited" {
			t.Fatalf("%s was edited", code)
		}
	}
}

func TestRuleErrors_AStatusErrorDoesNotClaimTheKindOrTitleIsMissing(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	d := mustCreateRule(t, mgr, "decision", "d")
	_, statusErr := mgr.SetRuleStatus(ctx, "owner", d.Code, "retired")
	_, priorityErr := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "x", Priority: "P1"})
	_, listErr := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "decision", Statuses: []string{"retired"}})
	for _, err := range []error{statusErr, priorityErr, listErr} {
		if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) || strings.Contains(err.Error(), "needs a kind") {
			t.Fatalf("err %q", err)
		}
	}
	_, bare := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule"})
	if !errors.Is(bare, mwanachamataskmanager.ErrInvalidRule) || !strings.Contains(bare.Error(), "needs a kind") {
		t.Fatalf("err %q", bare)
	}
}

func TestQuestions_DroppingAnAnsweredQuestionClearsItsAnswer(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P1")
	d := mustCreateRule(t, mgr, "decision", "d")
	if _, err := mgr.AnswerQuestion(ctx, "owner", q.Code, d.Code); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.SetRuleStatus(ctx, "owner", q.Code, "dropped")
	if err != nil || got.Status != "dropped" || got.AnsweredBy != "" {
		t.Fatalf("got %+v, err %v", got, err)
	}
	if mustReadRule(t, mgr, q.Code).AnsweredBy != "" {
		t.Fatal("stored answer kept")
	}
}

func TestRuleList_ATagFilterFindsEveryMatchPastTheRowLimit(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	for i := 0; i < 505; i++ {
		mustCreateRule(t, mgr, "rule", fmt.Sprintf("plain %d", i))
	}
	want := map[string]bool{}
	for i := 0; i < 3; i++ {
		r, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "tagged", Tags: []string{"Needle"}})
		if err != nil {
			t.Fatal(err)
		}
		want[r.Code] = true
	}
	got, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Tags: []string{"needle"}})
	if err != nil || len(got) != 3 {
		t.Fatalf("%d found, err %v, want 3", len(got), err)
	}
	for _, r := range got {
		if !want[r.Code] {
			t.Fatalf("unexpected %s", r.Code)
		}
	}
	all, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "rule"})
	if err != nil || len(all) != 508 {
		t.Fatalf("untagged list %d, err %v, want 508", len(all), err)
	}
}

func TestRuleList_BlankAndPaddedFilterTagsAreTrimmedAndDropped(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	tagged, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "a", Tags: []string{"css"}})
	if err != nil {
		t.Fatal(err)
	}
	mustCreateRule(t, mgr, "rule", "b")
	got, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Tags: []string{"  CSS ", "", "  "}})
	if err != nil || codes(got) != tagged.Code {
		t.Fatalf("got %q, err %v, want %s", codes(got), err, tagged.Code)
	}
	everything, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Tags: []string{"", " "}})
	if err != nil || len(everything) != 2 {
		t.Fatalf("blank tags: %d, err %v, want 2", len(everything), err)
	}
}
