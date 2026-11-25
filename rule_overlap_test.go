package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func ruleInForceWhen(t *testing.T, mgr mwanachamataskmanager.TaskManager, title, appliesWhen string) mwanachamataskmanager.RuleRecord {
	t.Helper()
	ctx := context.Background()
	rule := mwanachamataskmanager.RuleRecord{Kind: "rule", Title: title}
	if appliesWhen != "" {
		rule.AppliesWhen = json.RawMessage(appliesWhen)
	}
	created, err := mgr.CreateRule(ctx, "owner", rule)
	if err != nil {
		t.Fatal(err)
	}
	created, err = mgr.SetRuleStatus(ctx, "owner", created.Code, "in_force")
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func tagWhen(tag string) string {
	return `{"match":"all","rules":[{"field":"tag","operator":"has","value":"` + tag + `"}]}`
}

func overlaps(t *testing.T, mgr mwanachamataskmanager.TaskManager) []mwanachamataskmanager.RuleOverlap {
	t.Helper()
	out, err := mgr.ListRuleOverlaps(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRuleOverlap_TwoInForceRulesWithTheSameSetAreReported(t *testing.T) {
	mgr := newTestManager(t)
	a := ruleInForceWhen(t, mgr, "Use rem", cssWhen)
	b := ruleInForceWhen(t, mgr, "Use px", `{"match":"all","rules":[{"field":"TAG","operator":"has","value":"css"}]}`)

	got := overlaps(t, mgr)

	if len(got) != 1 || got[0].First.Code != a.Code || got[0].Second.Code != b.Code {
		t.Fatalf("overlaps %+v", got)
	}
	if len(got[0].Reasons) != 1 || got[0].Reasons[0] != mwanachamataskmanager.OverlapSharedKey || got[0].SharedKey == "" {
		t.Fatalf("reasons %+v", got[0])
	}
}

func TestRuleOverlap_OnlyInForceRulesWithASetCount(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	ruleInForceWhen(t, mgr, "Has a set", cssWhen)
	ruleInForceWhen(t, mgr, "Has no set", "")
	proposed, _ := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "Proposed", AppliesWhen: json.RawMessage(cssWhen)})
	retired := ruleInForceWhen(t, mgr, "Retired", cssWhen)
	if _, err := mgr.SetRuleStatus(ctx, "owner", retired.Code, "retired"); err != nil {
		t.Fatal(err)
	}
	decision, _ := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "decision", Title: "Decided"})
	if _, err := mgr.SetRuleStatus(ctx, "owner", decision.Code, "decided"); err != nil {
		t.Fatal(err)
	}
	_ = proposed

	if got := overlaps(t, mgr); len(got) != 0 {
		t.Fatalf("overlaps %+v", got)
	}
}

func TestRuleOverlap_ASupersededPairIsNotReported(t *testing.T) {
	mgr := newTestManager(t)
	old := ruleInForceWhen(t, mgr, "Old", cssWhen)
	next, err := mgr.SupersedeRule(context.Background(), "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "New", AppliesWhen: json.RawMessage(cssWhen)})
	if err != nil {
		t.Fatal(err)
	}
	if got := overlaps(t, mgr); len(got) != 0 {
		t.Fatalf("overlaps %+v for %s", got, next.Code)
	}
}

func TestRuleOverlap_ThreeRulesGiveEveryPairInCodeOrder(t *testing.T) {
	mgr := newTestManager(t)
	a := ruleInForceWhen(t, mgr, "A", cssWhen)
	b := ruleInForceWhen(t, mgr, "B", cssWhen)
	c := ruleInForceWhen(t, mgr, "C", cssWhen)

	got := overlaps(t, mgr)

	want := [][2]string{{a.Code, b.Code}, {a.Code, c.Code}, {b.Code, c.Code}}
	if len(got) != len(want) {
		t.Fatalf("overlaps %+v", got)
	}
	for i, pair := range want {
		if got[i].First.Code != pair[0] || got[i].Second.Code != pair[1] {
			t.Fatalf("pair %d is %s %s, want %v", i, got[i].First.Code, got[i].Second.Code, pair)
		}
	}
}
