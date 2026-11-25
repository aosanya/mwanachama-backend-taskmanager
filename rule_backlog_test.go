package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type backlogRuleFixture struct {
	mgr     mwanachamataskmanager.TaskManager
	backlog string
}

func newBacklogRuleFixture(t *testing.T) backlogRuleFixture {
	t.Helper()
	ctx := context.Background()
	mgr := newTestManager(t)
	made, err := mgr.CreateBacklog(ctx, "Sprint 1", []string{"Open", "Done"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tags := range [][]string{{"Style"}, {"data", "style"}} {
		if _, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "an issue", Backlog: made.ID, Tags: tags}); err != nil {
			t.Fatal(err)
		}
	}
	return backlogRuleFixture{mgr: mgr, backlog: made.ID}
}

func (f backlogRuleFixture) record(t *testing.T, kind, title string, tags ...string) mwanachamataskmanager.RuleRecord {
	t.Helper()
	made, err := f.mgr.CreateRule(context.Background(), "owner", mwanachamataskmanager.RuleRecord{Kind: kind, Title: title, Tags: tags})
	if err != nil {
		t.Fatal(err)
	}
	return made
}

func TestBacklogRules_ListsOnlyTheRulesSharingATagWithItsIssues(t *testing.T) {
	f := newBacklogRuleFixture(t)
	matched := f.record(t, "rule", "matched by style", "style")
	other := f.record(t, "rule", "matched by data", "DATA")
	f.record(t, "rule", "unrelated", "infra")
	f.record(t, "rule", "no tags")
	f.record(t, "decision", "a decision on style", "style")

	got, err := f.mgr.ListBacklogRules(context.Background(), "owner", f.backlog, mwanachamataskmanager.RuleFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if codes(got) != matched.Code+","+other.Code {
		t.Fatalf("got %q, want %s,%s", codes(got), matched.Code, other.Code)
	}
}

func TestBacklogRules_ASupersededRuleIsHiddenUnlessAskedFor(t *testing.T) {
	ctx := context.Background()
	f := newBacklogRuleFixture(t)
	old := f.record(t, "rule", "old", "style")
	mustSetStatus(t, f.mgr, old.Code, "in_force")
	replacement, err := f.mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "new", Tags: []string{"style"}})
	if err != nil {
		t.Fatal(err)
	}
	retired := f.record(t, "rule", "retired", "style")
	mustSetStatus(t, f.mgr, retired.Code, "in_force")
	mustSetStatus(t, f.mgr, retired.Code, "retired")

	got, err := f.mgr.ListBacklogRules(ctx, "owner", f.backlog, mwanachamataskmanager.RuleFilter{})
	if err != nil || codes(got) != replacement.Code {
		t.Fatalf("default %q, err %v, want %s", codes(got), err, replacement.Code)
	}
	all, err := f.mgr.ListBacklogRules(ctx, "owner", f.backlog, mwanachamataskmanager.RuleFilter{Statuses: []string{"superseded", "retired", "in_force"}})
	if err != nil || len(all) != 3 {
		t.Fatalf("asked for: %q, err %v, want 3", codes(all), err)
	}
}

func TestBacklogRules_AStatusOfADecisionIsRefused(t *testing.T) {
	f := newBacklogRuleFixture(t)
	_, err := f.mgr.ListBacklogRules(context.Background(), "owner", f.backlog, mwanachamataskmanager.RuleFilter{Statuses: []string{"decided"}})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}

func TestBacklogRules_AnUnknownBacklogIsRefusedAndAnEmptyOneListsNothing(t *testing.T) {
	ctx := context.Background()
	f := newBacklogRuleFixture(t)
	f.record(t, "rule", "style rule", "style")
	if _, err := f.mgr.ListBacklogRules(ctx, "owner", "nope", mwanachamataskmanager.RuleFilter{}); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("err %v, want ErrUnknownBacklog", err)
	}
	empty, err := f.mgr.CreateBacklog(ctx, "Empty", []string{"Open"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.mgr.ListBacklogRules(ctx, "owner", empty.ID, mwanachamataskmanager.RuleFilter{})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %q, err %v, want none", codes(got), err)
	}
}

func TestBacklogDecisions_ListsOnlyTheDecisionsSharingATagWithItsIssues(t *testing.T) {
	f := newBacklogRuleFixture(t)
	matched := f.record(t, "decision", "matched", "Style")
	f.record(t, "decision", "unrelated", "infra")
	f.record(t, "rule", "a rule on style", "style")

	got, err := f.mgr.ListBacklogDecisions(context.Background(), "owner", f.backlog, mwanachamataskmanager.RuleFilter{})
	if err != nil || codes(got) != matched.Code {
		t.Fatalf("got %q, err %v, want %s", codes(got), err, matched.Code)
	}
}

func TestBacklogDecisions_DefaultIsDecidedAndProposedAndSupersededNeedsAsking(t *testing.T) {
	ctx := context.Background()
	f := newBacklogRuleFixture(t)
	proposed := f.record(t, "decision", "proposed", "style")
	decided := f.record(t, "decision", "decided", "style")
	mustSetStatus(t, f.mgr, decided.Code, "decided")
	old := f.record(t, "decision", "old", "style")
	mustSetStatus(t, f.mgr, old.Code, "decided")
	replacement, err := f.mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "new", Tags: []string{"style"}})
	if err != nil {
		t.Fatal(err)
	}

	got, err := f.mgr.ListBacklogDecisions(ctx, "owner", f.backlog, mwanachamataskmanager.RuleFilter{})
	if err != nil || codes(got) != proposed.Code+","+decided.Code+","+replacement.Code {
		t.Fatalf("default %q, err %v", codes(got), err)
	}
	asked, err := f.mgr.ListBacklogDecisions(ctx, "owner", f.backlog, mwanachamataskmanager.RuleFilter{Statuses: []string{"superseded"}})
	if err != nil || codes(asked) != old.Code {
		t.Fatalf("superseded %q, err %v, want %s", codes(asked), err, old.Code)
	}
	if _, err := f.mgr.ListBacklogDecisions(ctx, "owner", f.backlog, mwanachamataskmanager.RuleFilter{Statuses: []string{"in_force"}}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
}

func TestBacklogQuestions_ListsOnlyOpenQuestionsSharingATagP1First(t *testing.T) {
	ctx := context.Background()
	f := newBacklogRuleFixture(t)
	plain := f.record(t, "question", "plain", "style")
	urgent, err := f.mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "question", Title: "urgent", Priority: "P1", Tags: []string{"data"}})
	if err != nil {
		t.Fatal(err)
	}
	f.record(t, "question", "elsewhere", "infra")
	f.record(t, "rule", "a rule on style", "style")
	settled := f.record(t, "question", "settled", "style")
	answer := f.record(t, "decision", "answer", "style")
	if _, err := f.mgr.AnswerQuestion(ctx, "owner", settled.Code, answer.Code); err != nil {
		t.Fatal(err)
	}

	got, err := f.mgr.ListBacklogQuestions(ctx, "owner", f.backlog, mwanachamataskmanager.RuleFilter{})
	if err != nil || codes(got) != urgent.Code+","+plain.Code {
		t.Fatalf("open %q, err %v, want %s,%s", codes(got), err, urgent.Code, plain.Code)
	}
	answered, err := f.mgr.ListBacklogQuestions(ctx, "owner", f.backlog, mwanachamataskmanager.RuleFilter{Statuses: []string{"answered"}})
	if err != nil || codes(answered) != settled.Code {
		t.Fatalf("answered %q, err %v", codes(answered), err)
	}
	if _, err := f.mgr.ListBacklogQuestions(ctx, "owner", f.backlog, mwanachamataskmanager.RuleFilter{Statuses: []string{"in_force"}}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("err %v, want ErrInvalidRule", err)
	}
	if _, err := f.mgr.ListBacklogQuestions(ctx, "owner", "nope", mwanachamataskmanager.RuleFilter{}); !errors.Is(err, mwanachamataskmanager.ErrUnknownBacklog) {
		t.Fatalf("err %v, want ErrUnknownBacklog", err)
	}
}
