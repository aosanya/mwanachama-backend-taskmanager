package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestQuestionEdit_OptionsAndRoleChangeAtAnyTimeEvenAfterAnAnswer(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	q := askedWithOptions(t, mgr)
	if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, optionAt(0)); err != nil {
		t.Fatal(err)
	}
	options := []mwanachamataskmanager.QuestionOption{{Text: "Yes"}, {Text: "No", Recommended: true, Reason: "Safer."}}
	role := "Council"
	got, err := mgr.UpdateRule(ctx, "owner", q.Code, mwanachamataskmanager.RulePatch{Options: &options, TargetRole: &role})
	if err != nil {
		t.Fatal(err)
	}
	if got.TargetRole != "Council" || len(got.Options) != 2 || got.Options[0].Text != "No" || got.Status != "answered" || got.Answer != "Spaces" {
		t.Fatalf("got %+v", got)
	}
	read, err := mgr.ReadRule(ctx, "owner", q.Code)
	if err != nil || read.TargetRole != "Council" || len(read.Options) != 2 {
		t.Fatalf("read %+v, %v", read, err)
	}
}

func TestQuestionEdit_TheSameRulesHoldAsAtCreation(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	q := askedWithOptions(t, mgr)
	blank := "  "
	two := []mwanachamataskmanager.QuestionOption{{Text: "A", Recommended: true, Reason: "r"}, {Text: "B", Recommended: true, Reason: "r"}}
	noReason := []mwanachamataskmanager.QuestionOption{{Text: "A", Recommended: true}}
	for name, patch := range map[string]mwanachamataskmanager.RulePatch{
		"a blank role":    {TargetRole: &blank},
		"two recommended": {Options: &two},
		"no reason":       {Options: &noReason},
	} {
		if _, err := mgr.UpdateRule(ctx, "owner", q.Code, patch); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s: err %v, want ErrInvalidRule", name, err)
		}
	}
}

func TestQuestionEdit_ARuleOrDecisionTakesNeitherRoleNorOptions(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	role := "Council"
	options := []mwanachamataskmanager.QuestionOption{{Text: "x"}}
	for _, kind := range []string{"rule", "decision"} {
		rec := mustCreateRule(t, mgr, kind, "A "+kind)
		for name, patch := range map[string]mwanachamataskmanager.RulePatch{"a role": {TargetRole: &role}, "options": {Options: &options}} {
			if _, err := mgr.UpdateRule(ctx, "owner", rec.Code, patch); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
				t.Fatalf("%s with %s: err %v, want ErrInvalidRule", kind, name, err)
			}
		}
	}
}

func TestQuestionEdit_AnEditKeepsTheOptimisticStampRule(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	q := askedWithOptions(t, mgr)
	stale := "2000-01-01T00:00:00Z"
	options := []mwanachamataskmanager.QuestionOption{{Text: "Only"}}
	_, err := mgr.UpdateRule(ctx, "owner", q.Code, mwanachamataskmanager.RulePatch{Options: &options, ExpectedUpdatedAt: &stale})
	if !errors.Is(err, mwanachamataskmanager.ErrRuleStale) {
		t.Fatalf("err %v, want ErrRuleStale", err)
	}
}
