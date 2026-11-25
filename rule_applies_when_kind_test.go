package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func wantInvalidRule(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("%s: got %v, want ErrInvalidRule", what, err)
	}
}

func TestAppliesWhen_IsRefusedOnADecisionAndOnAQuestion(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	for _, kind := range []string{"decision", "question"} {
		_, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: kind, Title: "x", AppliesWhen: json.RawMessage(cssWhen)})
		wantInvalidRule(t, "create a "+kind, err)

		plain := mustCreateRule(t, mgr, kind, "plain "+kind)
		set := json.RawMessage(cssWhen)
		_, err = mgr.UpdateRule(ctx, "owner", plain.Code, mwanachamataskmanager.RulePatch{AppliesWhen: &set})
		wantInvalidRule(t, "update a "+kind, err)
		read, err := mgr.GetRuleByCode(ctx, "owner", plain.Code)
		if err != nil || len(read.AppliesWhen) != 0 {
			t.Fatalf("%s after a refused update: %+v, %v", kind, read, err)
		}
	}
}

func TestAppliesWhen_IsRefusedWhenSupersedingADecision(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := mustCreateRule(t, mgr, "decision", "Old choice")
	mustSetStatus(t, mgr, old.Code, "decided")

	_, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "New choice", AppliesWhen: json.RawMessage(cssWhen)})

	wantInvalidRule(t, "supersede a decision", err)
	read, err := mgr.GetRuleByCode(ctx, "owner", old.Code)
	if err != nil || read.Status != "decided" {
		t.Fatalf("the refused supersede changed the decision: %+v, %v", read, err)
	}
}

func TestAppliesWhen_AnEmptyObjectOnADecisionIsNotASet(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := mgr.CreateRule(context.Background(), "owner", mwanachamataskmanager.RuleRecord{Kind: "decision", Title: "x", AppliesWhen: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
}

func TestAppliesWhen_ABodyWithNoMatchAndNoRulesIsTreatedAsEmpty(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "x", AppliesWhen: json.RawMessage(`{"rules":[]}`)})
	if err != nil || len(made.AppliesWhen) != 0 {
		t.Fatalf("created %+v, %v", made, err)
	}
	set := json.RawMessage(cssWhen)
	if _, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{AppliesWhen: &set}); err != nil {
		t.Fatal(err)
	}
	empty := json.RawMessage(`{"rules":[]}`)
	cleared, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{AppliesWhen: &empty})
	if err != nil || len(cleared.AppliesWhen) != 0 {
		t.Fatalf("cleared %+v, %v", cleared, err)
	}
}
