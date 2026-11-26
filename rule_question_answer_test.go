package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func optionAt(i int) mwanachamataskmanager.QuestionAnswer {
	return mwanachamataskmanager.QuestionAnswer{Option: &i}
}

func askedWithOptions(t *testing.T, mgr mwanachamataskmanager.TaskManager) mwanachamataskmanager.RuleRecord {
	t.Helper()
	return mustAsk(t, mgr, mwanachamataskmanager.RuleRecord{Title: "Tabs or spaces?", Options: threeOptions()})
}

func countOfKind(t *testing.T, mgr mwanachamataskmanager.TaskManager, kind string) int {
	t.Helper()
	got, err := mgr.ListRules(context.Background(), "owner", mwanachamataskmanager.RuleFilter{Kind: kind, Statuses: []string{"proposed", "decided"}})
	if err != nil {
		t.Fatal(err)
	}
	return len(got)
}

func TestQuestionAnswer_AnOptionClosesTheQuestionWithItsTextAndNoDecision(t *testing.T) {
	mgr := newTestManager(t)
	q := askedWithOptions(t, mgr)
	got, err := mgr.AnswerQuestionWith(context.Background(), "owner", q.Code, optionAt(0))
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "answered" || got.Answer != "Spaces" || got.AnsweredBy != "" {
		t.Fatalf("got %+v", got)
	}
	if n := countOfKind(t, mgr, "decision"); n != 0 {
		t.Fatalf("%d decisions recorded, want none", n)
	}
	read, err := mgr.ReadRule(context.Background(), "owner", q.Code)
	if err != nil || read.Status != "answered" || read.Answer != "Spaces" {
		t.Fatalf("read %+v, %v", read, err)
	}
}

func TestQuestionAnswer_TypedWordsCloseTheQuestionAsOther(t *testing.T) {
	mgr := newTestManager(t)
	q := mustAsk(t, mgr, mwanachamataskmanager.RuleRecord{Title: "Which?"})
	got, err := mgr.AnswerQuestionWith(context.Background(), "owner", q.Code, mwanachamataskmanager.QuestionAnswer{Other: "  Neither; ask the council.  "})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "answered" || got.Answer != "Neither; ask the council." || got.AnsweredBy != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestQuestionAnswer_ADecisionCodeStillSettlesItAsBefore(t *testing.T) {
	mgr := newTestManager(t)
	q := askedWithOptions(t, mgr)
	d := mustCreateRule(t, mgr, "decision", "Spaces")
	got, err := mgr.AnswerQuestionWith(context.Background(), "owner", q.Code, mwanachamataskmanager.QuestionAnswer{DecisionCode: d.Code})
	if err != nil || got.Status != "answered" || got.AnsweredBy != d.Code || got.Answer != "" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestQuestionAnswer_ExactlyOneWayOfAnsweringIsGiven(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := askedWithOptions(t, mgr)
	d := mustCreateRule(t, mgr, "decision", "Spaces")
	zero, outOfRange, negative := 0, 3, -1
	cases := map[string]mwanachamataskmanager.QuestionAnswer{
		"nothing":                   {},
		"blank words":               {Other: "   "},
		"an option and words":       {Option: &zero, Other: "x"},
		"an option and a decision":  {Option: &zero, DecisionCode: d.Code},
		"words and a decision":      {Other: "x", DecisionCode: d.Code},
		"an index past the options": {Option: &outOfRange},
		"a negative index":          {Option: &negative},
	}
	for name, answer := range cases {
		if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, answer); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s: err %v, want ErrInvalidRule", name, err)
		}
	}
	bare := mustAsk(t, mgr, mwanachamataskmanager.RuleRecord{Title: "No options"})
	if _, err := mgr.AnswerQuestionWith(ctx, "owner", bare.Code, optionAt(0)); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("an option on a question with none: err %v, want ErrInvalidRule", err)
	}
	if read, _ := mgr.ReadRule(ctx, "owner", q.Code); read.Status != "open" {
		t.Fatalf("a refused answer left the question %s", read.Status)
	}
}

func TestQuestionAnswer_OnlyTheTargetRoleAnswers(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	holds := map[string]bool{"ann": true}
	mgr.UseRoleHolder(func(_ context.Context, caller, role string) (bool, error) {
		return role == askedRole && holds[caller], nil
	})
	d := mustCreateRule(t, mgr, "decision", "Spaces")
	for name, answer := range map[string]mwanachamataskmanager.QuestionAnswer{
		"an option":  optionAt(0),
		"words":      {Other: "x"},
		"a decision": {DecisionCode: d.Code},
	} {
		q := askedWithOptions(t, mgr)
		if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, answer); !errors.Is(err, mwanachamataskmanager.ErrBacklogForbidden) {
			t.Fatalf("%s by a caller without the role: err %v, want ErrBacklogForbidden", name, err)
		}
		if read, _ := mgr.ReadRule(ctx, "owner", q.Code); read.Status != "open" {
			t.Fatalf("%s: a refused answer left the question %s", name, read.Status)
		}
		if got, err := mgr.AnswerQuestionWith(ctx, "ann", q.Code, answer); err != nil || got.Status != "answered" {
			t.Fatalf("%s by a holder: %+v, %v", name, got, err)
		}
	}
}

func TestQuestionAnswer_TheOldDecisionMethodChecksTheRoleToo(t *testing.T) {
	mgr := newTestManager(t)
	mgr.UseRoleHolder(func(context.Context, string, string) (bool, error) { return false, nil })
	q := askedWithOptions(t, mgr)
	d := mustCreateRule(t, mgr, "decision", "Spaces")
	if _, err := mgr.AnswerQuestion(context.Background(), "owner", q.Code, d.Code); !errors.Is(err, mwanachamataskmanager.ErrBacklogForbidden) {
		t.Fatalf("err %v, want ErrBacklogForbidden", err)
	}
}

func TestQuestionAnswer_NoRoleCheckWithoutAHookOrWithoutACaller(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	if got, err := mgr.AnswerQuestionWith(ctx, "owner", askedWithOptions(t, mgr).Code, optionAt(2)); err != nil || got.Answer != "Leave it to each author" {
		t.Fatalf("no hook: %+v, %v", got, err)
	}
	mgr.UseRoleHolder(func(context.Context, string, string) (bool, error) { return false, nil })
	if _, err := mgr.AnswerQuestionWith(ctx, "", askedWithOptions(t, mgr).Code, optionAt(0)); err != nil {
		t.Fatalf("no caller: %v", err)
	}
}

func TestQuestionAnswer_ReopeningOrDroppingClearsTheAnswer(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := askedWithOptions(t, mgr)
	if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, optionAt(1)); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"open", "dropped"} {
		if status == "dropped" {
			if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, optionAt(1)); err != nil {
				t.Fatal(err)
			}
		}
		got, err := mgr.SetRuleStatus(ctx, "owner", q.Code, status)
		if err != nil || got.Answer != "" || got.AnsweredBy != "" {
			t.Fatalf("%s: %+v, %v", status, got, err)
		}
	}
}
