package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func overlappingPair(t *testing.T) (mwanachamataskmanager.TaskManager, mwanachamataskmanager.RuleRecord, mwanachamataskmanager.RuleRecord) {
	t.Helper()
	mgr := newTestManager(t)
	a := ruleInForceWhen(t, mgr, "a", tagWhen("css"))
	b := ruleInForceWhen(t, mgr, "b", tagWhen("css"))
	return mgr, a, b
}

func TestCompatible_APairJudgedCompatibleLeavesTheOverlapReadAndIsCounted(t *testing.T) {
	ctx := context.Background()
	mgr, a, b := overlappingPair(t)
	if got := overlaps(t, mgr); len(got) != 1 {
		t.Fatalf("before: %+v", got)
	}

	if err := mgr.AddRuleCompatible(ctx, "owner", a.Code, b.Code); err != nil {
		t.Fatal(err)
	}

	list, err := mgr.ListRuleOverlaps(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 0 || list.HiddenCompatible != 1 {
		t.Fatalf("after: %+v", list)
	}
}

func TestCompatible_ThePairIsFoundFromEitherSideAndRemovedFromEither(t *testing.T) {
	ctx := context.Background()
	mgr, a, b := overlappingPair(t)
	if err := mgr.AddRuleCompatible(ctx, "owner", b.Code, a.Code); err != nil {
		t.Fatal(err)
	}

	fromA, err := mgr.ReadRule(ctx, "owner", a.Code)
	if err != nil {
		t.Fatal(err)
	}
	fromB, err := mgr.ReadRule(ctx, "owner", b.Code)
	if err != nil {
		t.Fatal(err)
	}
	if len(fromA.Compatible) != 1 || fromA.Compatible[0] != b.Code || len(fromB.Compatible) != 1 || fromB.Compatible[0] != a.Code {
		t.Fatalf("a %v b %v", fromA.Compatible, fromB.Compatible)
	}

	if err := mgr.RemoveRuleCompatible(ctx, "owner", a.Code, b.Code); err != nil {
		t.Fatal(err)
	}
	list, _ := mgr.ListRuleOverlaps(ctx, "owner")
	if len(list.Items) != 1 || list.HiddenCompatible != 0 {
		t.Fatalf("after remove: %+v", list)
	}
}

func TestCompatible_ARuleWithNoPairsReadsAnEmptyList(t *testing.T) {
	mgr, a, _ := overlappingPair(t)
	got, err := mgr.ReadRule(context.Background(), "owner", a.Code)
	if err != nil {
		t.Fatal(err)
	}
	if got.Compatible == nil || len(got.Compatible) != 0 || got.Code != a.Code {
		t.Fatalf("got %+v", got)
	}
}

func TestCompatible_RefusalsAreNamed(t *testing.T) {
	ctx := context.Background()
	mgr, a, b := overlappingPair(t)
	decision := mustCreateRule(t, mgr, "decision", "Use X")
	question := mustCreateRule(t, mgr, "question", "Why?")

	for name, other := range map[string]string{"self": a.Code, "decision": decision.Code, "question": question.Code} {
		if err := mgr.AddRuleCompatible(ctx, "owner", a.Code, other); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := mgr.AddRuleCompatible(ctx, "owner", decision.Code, a.Code); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Errorf("decision first: %v", err)
	}
	if err := mgr.AddRuleCompatible(ctx, "owner", a.Code, "R-99"); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Errorf("unknown: %v", err)
	}
	if err := mgr.AddRuleCompatible(ctx, "owner", a.Code, b.Code); err != nil {
		t.Fatal(err)
	}
	if err := mgr.AddRuleCompatible(ctx, "owner", b.Code, a.Code); !errors.Is(err, mwanachamataskmanager.ErrDuplicateRuleCompatible) {
		t.Errorf("duplicate: %v", err)
	}
	if err := mgr.RemoveRuleCompatible(ctx, "owner", a.Code, decision.Code); !errors.Is(err, mwanachamataskmanager.ErrRuleCompatibleNotFound) {
		t.Errorf("remove missing: %v", err)
	}
}

func TestCompatible_ARetiredRulesPairStaysButNoLongerShows(t *testing.T) {
	ctx := context.Background()
	mgr, a, b := overlappingPair(t)
	if err := mgr.AddRuleCompatible(ctx, "owner", a.Code, b.Code); err != nil {
		t.Fatal(err)
	}
	activate(t, mgr, b.Code, "retired")

	got, _ := mgr.ReadRule(ctx, "owner", a.Code)
	if len(got.Compatible) != 0 {
		t.Fatalf("retired shown: %v", got.Compatible)
	}
	list, _ := mgr.ListRuleOverlaps(ctx, "owner")
	if len(list.Items) != 0 || list.HiddenCompatible != 0 {
		t.Fatalf("overlap: %+v", list)
	}
	activate(t, mgr, b.Code, "in_force")
	got, _ = mgr.ReadRule(ctx, "owner", a.Code)
	if len(got.Compatible) != 1 {
		t.Fatalf("pair lost: %v", got.Compatible)
	}
}

func TestCompatible_OnlyThePairIsHiddenNotTheOthers(t *testing.T) {
	ctx := context.Background()
	mgr, a, b := overlappingPair(t)
	c := ruleInForceWhen(t, mgr, "c", tagWhen("css"))
	if err := mgr.AddRuleCompatible(ctx, "owner", a.Code, b.Code); err != nil {
		t.Fatal(err)
	}
	list, _ := mgr.ListRuleOverlaps(ctx, "owner")
	if len(list.Items) != 2 || list.HiddenCompatible != 1 {
		t.Fatalf("got %+v (c=%s)", list, c.Code)
	}
}
