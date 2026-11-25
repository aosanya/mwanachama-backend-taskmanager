package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func mustCreateQuestion(t *testing.T, mgr mwanachamataskmanager.TaskManager, title, priority string) mwanachamataskmanager.RuleRecord {
	t.Helper()
	q, err := mgr.CreateRule(context.Background(), "owner", mwanachamataskmanager.RuleRecord{Kind: "question", Title: title, Priority: priority})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestQuestions_AQuestionHasItsOwnCodeSeriesAndStartsOpen(t *testing.T) {
	mgr := newTestManager(t)
	mustCreateRule(t, mgr, "rule", "A rule")
	mustCreateRule(t, mgr, "decision", "A decision")
	first := mustCreateQuestion(t, mgr, "Does a bad example sit beside the good one?", "P1")
	second := mustCreateQuestion(t, mgr, "How does a printed PDF show file links?", "P2")
	if first.Code != "Q-1" || second.Code != "Q-2" {
		t.Fatalf("codes %q and %q, want Q-1 and Q-2", first.Code, second.Code)
	}
	if first.Status != "open" {
		t.Fatalf("status %q, want open", first.Status)
	}
}

func TestQuestions_ACodeIsNeverGivenAgainAfterADrop(t *testing.T) {
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Asked once", "P3")
	mustSetStatus(t, mgr, q.Code, "dropped")
	next := mustCreateQuestion(t, mgr, "Asked again", "P3")
	if next.Code != "Q-2" {
		t.Fatalf("code %q, want Q-2", next.Code)
	}
}

func TestQuestions_StatusIsOpenAnsweredOrDropped(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P2")
	for _, bad := range []string{"proposed", "in_force", "decided", "retired", "superseded"} {
		if _, err := mgr.SetRuleStatus(ctx, "owner", q.Code, bad); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s: err %v, want ErrInvalidRule", bad, err)
		}
	}
	if got, err := mgr.SetRuleStatus(ctx, "owner", q.Code, "dropped"); err != nil || got.Status != "dropped" {
		t.Fatalf("dropped: %+v, %v", got, err)
	}
	if got, err := mgr.SetRuleStatus(ctx, "owner", q.Code, "open"); err != nil || got.Status != "open" {
		t.Fatalf("reopened: %+v, %v", got, err)
	}
	rule := mustCreateRule(t, mgr, "rule", "r")
	for _, bad := range []string{"open", "answered", "dropped"} {
		if _, err := mgr.SetRuleStatus(ctx, "owner", rule.Code, bad); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("rule %s: err %v, want ErrInvalidRule", bad, err)
		}
	}
}

func TestQuestions_AnAnswerIsGivenByAnswerNotByStatus(t *testing.T) {
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "x", "P1")
	if _, err := mgr.SetRuleStatus(context.Background(), "owner", q.Code, "answered"); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}

func TestQuestions_PriorityIsP1ToP3OnQuestionsOnly(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	for _, p := range []string{"P1", "P2", "P3"} {
		if q := mustCreateQuestion(t, mgr, "q "+p, p); q.Priority != p {
			t.Fatalf("priority %q, want %s", q.Priority, p)
		}
	}
	if q := mustCreateQuestion(t, mgr, "no priority given", ""); q.Priority != "P2" {
		t.Fatalf("default priority %q, want P2", q.Priority)
	}
	for _, bad := range []string{"P0", "P4", "high", "p1"} {
		if _, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "question", Title: "x", Priority: bad}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s: err %v, want ErrInvalidRule", bad, err)
		}
	}
	for _, kind := range []string{"rule", "decision"} {
		if _, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: kind, Title: "x", Priority: "P1"}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s create: err %v, want ErrInvalidRule", kind, err)
		}
		rec := mustCreateRule(t, mgr, kind, "plain")
		p := "P1"
		if _, err := mgr.UpdateRule(ctx, "owner", rec.Code, mwanachamataskmanager.RulePatch{Priority: &p}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s update: err %v, want ErrInvalidRule", kind, err)
		}
		if got, _ := mgr.GetRuleByCode(ctx, "owner", rec.Code); got.Priority != "" {
			t.Fatalf("%s kept priority %q", kind, got.Priority)
		}
	}
}

func TestQuestions_UpdateChangesPriorityAndWordsButRefusesABadPriority(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Old", "P3")
	title, p := "New", "P1"
	got, err := mgr.UpdateRule(ctx, "owner", q.Code, mwanachamataskmanager.RulePatch{Title: &title, Priority: &p})
	if err != nil || got.Priority != "P1" || got.Title != "New" || got.Status != "open" {
		t.Fatalf("got %+v, err %v", got, err)
	}
	bad := "P9"
	if _, err := mgr.UpdateRule(ctx, "owner", q.Code, mwanachamataskmanager.RulePatch{Priority: &bad}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}

func TestQuestions_ListShowsOpenQuestionsP1FirstThenByCode(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q1 := mustCreateQuestion(t, mgr, "one", "P3")
	q2 := mustCreateQuestion(t, mgr, "two", "P1")
	q3 := mustCreateQuestion(t, mgr, "three", "P2")
	q4 := mustCreateQuestion(t, mgr, "four", "P1")
	dropped := mustCreateQuestion(t, mgr, "five", "P1")
	mustSetStatus(t, mgr, dropped.Code, "dropped")

	got, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "question"})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{q2.Code, q4.Code, q3.Code, q1.Code}, ",")
	if codes(got) != want {
		t.Fatalf("got %q, want %q", codes(got), want)
	}
	all, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "question", Statuses: []string{"open", "answered", "dropped"}})
	if err != nil || len(all) != 5 {
		t.Fatalf("all: %d, err %v", len(all), err)
	}
	if _, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "question", Statuses: []string{"in_force"}}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}

func TestQuestions_AnsweringRecordsTheDecisionAndSetsAnswered(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P1")
	d := mustCreateRule(t, mgr, "decision", "This one")

	got, err := mgr.AnswerQuestion(ctx, "owner", q.Code, d.Code)
	if err != nil || got.Status != "answered" || got.AnsweredBy != d.Code {
		t.Fatalf("got %+v, err %v", got, err)
	}
	read, err := mgr.GetRuleByCode(ctx, "owner", q.Code)
	if err != nil || read.AnsweredBy != d.Code || read.Status != "answered" {
		t.Fatalf("read %+v, err %v", read, err)
	}
	open, err := mgr.ListRules(ctx, "owner", mwanachamataskmanager.RuleFilter{Kind: "question"})
	if err != nil || len(open) != 0 {
		t.Fatalf("open after answer: %d, err %v", len(open), err)
	}
}

func TestQuestions_AnsweringNeedsAnExistingDecisionAndAQuestion(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P1")
	rule := mustCreateRule(t, mgr, "rule", "A rule")
	other := mustCreateQuestion(t, mgr, "Another", "P1")
	if _, err := mgr.AnswerQuestion(ctx, "owner", q.Code, "D-99"); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("unknown: err %v, want ErrRuleNotFound", err)
	}
	for _, code := range []string{rule.Code, other.Code} {
		if _, err := mgr.AnswerQuestion(ctx, "owner", q.Code, code); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s: err %v, want ErrInvalidRule", code, err)
		}
	}
	d := mustCreateRule(t, mgr, "decision", "d")
	if _, err := mgr.AnswerQuestion(ctx, "owner", rule.Code, d.Code); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("answering a rule: err %v, want ErrInvalidRule", err)
	}
	if _, err := mgr.AnswerQuestion(ctx, "owner", "Q-99", d.Code); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("unknown question: err %v, want ErrRuleNotFound", err)
	}
	mustSetStatus(t, mgr, q.Code, "dropped")
	if _, err := mgr.AnswerQuestion(ctx, "owner", q.Code, d.Code); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("dropped: err %v, want ErrInvalidRule", err)
	}
}

func TestQuestions_ReopeningClearsTheAnswer(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P1")
	d := mustCreateRule(t, mgr, "decision", "d")
	if _, err := mgr.AnswerQuestion(ctx, "owner", q.Code, d.Code); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.SetRuleStatus(ctx, "owner", q.Code, "open")
	if err != nil || got.Status != "open" || got.AnsweredBy != "" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestQuestions_AQuestionIsNeverSuperseded(t *testing.T) {
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P1")
	_, err := mgr.SupersedeRule(context.Background(), "owner", q.Code, mwanachamataskmanager.RuleRecord{Title: "Replacement"})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}

func TestQuestions_ListWithoutAKindKeepsRulesAndDecisionsAndAddsOpenQuestions(t *testing.T) {
	mgr := newTestManager(t)
	r := mustCreateRule(t, mgr, "rule", "r")
	d := mustCreateRule(t, mgr, "decision", "d")
	q := mustCreateQuestion(t, mgr, "q", "P1")
	got, err := mgr.ListRules(context.Background(), "owner", mwanachamataskmanager.RuleFilter{})
	if err != nil || codes(got) != d.Code+","+q.Code+","+r.Code {
		t.Fatalf("got %q, err %v", codes(got), err)
	}
}
