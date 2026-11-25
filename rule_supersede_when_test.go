package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func supersedeWith(t *testing.T, appliesWhen string) mwanachamataskmanager.RuleRecord {
	t.Helper()
	mgr := newTestManager(t)
	old := ruleInForceWhen(t, mgr, "Old", cssWhen)
	replacement := mwanachamataskmanager.RuleRecord{Title: "New"}
	if appliesWhen != "" {
		replacement.AppliesWhen = json.RawMessage(appliesWhen)
	}
	next, err := mgr.SupersedeRule(context.Background(), "owner", old.Code, replacement)
	if err != nil {
		t.Fatal(err)
	}
	read, err := mgr.GetRuleByCode(context.Background(), "owner", next.Code)
	if err != nil {
		t.Fatal(err)
	}
	return read
}

func TestSupersede_AReplacementWithoutAppliesWhenKeepsTheOldRulesSet(t *testing.T) {
	sameJSON(t, supersedeWith(t, "").AppliesWhen, cssWhenNormal)
}

func TestSupersede_ANullAppliesWhenKeepsTheOldRulesSet(t *testing.T) {
	sameJSON(t, supersedeWith(t, "null").AppliesWhen, cssWhenNormal)
}

func TestSupersede_AnEmptyObjectClearsTheOldRulesSet(t *testing.T) {
	if got := supersedeWith(t, "{}").AppliesWhen; len(got) != 0 {
		t.Fatalf("applies_when %s", got)
	}
}

func TestSupersede_AReplacementWithItsOwnSetReplacesTheOldOne(t *testing.T) {
	sameJSON(t, supersedeWith(t, tagWhen("app")).AppliesWhen, `{"match":"all","rules":[{"field":"tag","operator":"has","value":"app"}]}`)
}

func TestSupersede_TheCarriedSetKeepsTheReplacementInTheOverlapCheck(t *testing.T) {
	mgr := newTestManager(t)
	old := ruleInForceWhen(t, mgr, "Old", cssWhen)
	next, err := mgr.SupersedeRule(context.Background(), "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "New"})
	if err != nil {
		t.Fatal(err)
	}
	other := ruleInForceWhen(t, mgr, "Other", cssWhen)

	got := overlaps(t, mgr)

	if len(got) != 1 || got[0].First.Code != next.Code || got[0].Second.Code != other.Code {
		t.Fatalf("overlaps %+v", got)
	}
}
