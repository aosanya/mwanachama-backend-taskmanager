package mwanachamataskmanager_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const cssWhen = `{"match":"All","rules":[{"field":"Tag","operator":"Has","value":"CSS"}]}`
const cssWhenNormal = `{"match":"all","rules":[{"field":"tag","operator":"has","value":"css"}]}`

func jsonText(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var compact map[string]any
	if err := json.Unmarshal(raw, &compact); err != nil {
		t.Fatalf("applies_when %q: %v", raw, err)
	}
	out, _ := json.Marshal(compact)
	return string(out)
}

func sameJSON(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var wanted map[string]any
	if err := json.Unmarshal([]byte(want), &wanted); err != nil {
		t.Fatal(err)
	}
	wantText, _ := json.Marshal(wanted)
	if jsonText(t, got) != string(wantText) {
		t.Fatalf("applies_when %s, want %s", got, wantText)
	}
}

func TestAppliesWhen_ACreatedRuleReadsBackNormalised(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)

	created, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "Use rem", AppliesWhen: json.RawMessage(cssWhen)})
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, created.AppliesWhen, cssWhenNormal)

	read, err := mgr.GetRuleByCode(ctx, "owner", created.Code)
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, read.AppliesWhen, cssWhenNormal)
}

func TestAppliesWhen_ARuleWithoutOneReadsBackWithoutOne(t *testing.T) {
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "Plain")
	read, err := mgr.GetRuleByCode(context.Background(), "owner", rule.Code)
	if err != nil || len(read.AppliesWhen) != 0 {
		t.Fatalf("read %+v %v", read, err)
	}
}

func TestAppliesWhen_AnInvalidSetIsRefusedOnCreateAndUpdate(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "Plain")
	invalid := map[string]string{
		"unknown field":   `{"match":"all","rules":[{"field":"colour","operator":"is","value":"red"}]}`,
		"wrong operator":  `{"match":"all","rules":[{"field":"tag","operator":"is","value":"css"}]}`,
		"no rules":        `{"match":"all","rules":[]}`,
		"unknown match":   `{"match":"some","rules":[{"field":"tag","operator":"has","value":"css"}]}`,
		"not an object":   `[1,2]`,
		"unknown members": `{"match":"all","rules":[{"field":"tag","operator":"has","value":"css"}],"extra":1}`,
	}
	for name, raw := range invalid {
		t.Run(name, func(t *testing.T) {
			_, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "x", AppliesWhen: json.RawMessage(raw)})
			if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
				t.Fatalf("create: %v", err)
			}
			patch := json.RawMessage(raw)
			_, err = mgr.UpdateRule(ctx, "owner", rule.Code, mwanachamataskmanager.RulePatch{AppliesWhen: &patch})
			if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
				t.Fatalf("update: %v", err)
			}
		})
	}
}

func TestAppliesWhen_UpdateSetsChangesAndClearsIt(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "Plain")

	set := json.RawMessage(cssWhen)
	got, err := mgr.UpdateRule(ctx, "owner", rule.Code, mwanachamataskmanager.RulePatch{AppliesWhen: &set})
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, got.AppliesWhen, cssWhenNormal)

	title := "Plain again"
	got, err = mgr.UpdateRule(ctx, "owner", rule.Code, mwanachamataskmanager.RulePatch{Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, got.AppliesWhen, cssWhenNormal)

	clear := json.RawMessage(`{}`)
	got, err = mgr.UpdateRule(ctx, "owner", rule.Code, mwanachamataskmanager.RulePatch{AppliesWhen: &clear})
	if err != nil || len(got.AppliesWhen) != 0 {
		t.Fatalf("cleared %+v %v", got, err)
	}
	read, _ := mgr.GetRuleByCode(ctx, "owner", rule.Code)
	if len(read.AppliesWhen) != 0 {
		t.Fatalf("read after clear %s", read.AppliesWhen)
	}
}

func TestAppliesWhen_ASupersedingRuleCarriesItsOwn(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := mustCreateRule(t, mgr, "rule", "Old")
	if _, err := mgr.SetRuleStatus(ctx, "owner", old.Code, "in_force"); err != nil {
		t.Fatal(err)
	}
	next, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "New", AppliesWhen: json.RawMessage(cssWhen)})
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, next.AppliesWhen, cssWhenNormal)
	_, err = mgr.SupersedeRule(ctx, "owner", next.Code, mwanachamataskmanager.RuleRecord{Title: "Newer", AppliesWhen: json.RawMessage(`{"match":"x","rules":[]}`)})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("supersede with a bad set: %v", err)
	}
}
