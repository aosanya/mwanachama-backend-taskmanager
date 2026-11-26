package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func issueWithQuestions(t *testing.T) (mwanachamataskmanager.TaskManager, mwanachamataskmanager.Task, map[string]mwanachamataskmanager.RuleRecord) {
	t.Helper()
	ctx := context.Background()
	mgr := newTestManager(t)
	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Ship it"})
	if err != nil {
		t.Fatal(err)
	}
	asked := map[string]mwanachamataskmanager.RuleRecord{}
	for name, priority := range map[string]string{"open": "P2", "answered": "P1", "dropped": "P3"} {
		q := mustAsk(t, mgr, mwanachamataskmanager.RuleRecord{Title: name + " question", Priority: priority, Options: threeOptions()})
		if err := mgr.LinkRule(ctx, "owner", q.Code, issue.ID, "applies_to"); err != nil {
			t.Fatal(err)
		}
		asked[name] = q
	}
	if _, err := mgr.AnswerQuestionWith(ctx, "owner", asked["answered"].Code, optionAt(0)); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SetRuleStatus(ctx, "owner", asked["dropped"].Code, "dropped"); err != nil {
		t.Fatal(err)
	}
	return mgr, issue, asked
}

func questionCodes(rules mwanachamataskmanager.IssueRules) []string {
	var out []string
	for _, q := range rules.Questions {
		out = append(out, q.Code)
	}
	return out
}

func TestIssueQuestions_OpenIsTheDefaultAndStaysTheOnlyOneWithoutAStatus(t *testing.T) {
	mgr, issue, asked := issueWithQuestions(t)
	got, err := mgr.ListRulesOfIssueWith(context.Background(), "owner", issue.ID, mwanachamataskmanager.RuleFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if codes := questionCodes(got); len(codes) != 1 || codes[0] != asked["open"].Code {
		t.Fatalf("questions %v, want only %s", codes, asked["open"].Code)
	}
}

func TestIssueQuestions_StatusPicksWhichStatusesTheQuestionsListHolds(t *testing.T) {
	mgr, issue, asked := issueWithQuestions(t)
	ctx := context.Background()
	got, err := mgr.ListRulesOfIssueWith(ctx, "owner", issue.ID, mwanachamataskmanager.RuleFilter{Statuses: []string{"answered"}})
	if err != nil {
		t.Fatal(err)
	}
	if codes := questionCodes(got); len(codes) != 1 || codes[0] != asked["answered"].Code {
		t.Fatalf("answered only: %v", codes)
	}
	all, err := mgr.ListRulesOfIssueWith(ctx, "owner", issue.ID, mwanachamataskmanager.RuleFilter{Statuses: []string{"open", "answered", "dropped"}})
	if err != nil {
		t.Fatal(err)
	}
	codes := questionCodes(all)
	want := []string{asked["answered"].Code, asked["open"].Code, asked["dropped"].Code}
	if len(codes) != 3 || codes[0] != want[0] || codes[1] != want[1] || codes[2] != want[2] {
		t.Fatalf("all: %v, want P1 first %v", codes, want)
	}
}

func TestIssueQuestions_AQuestionIsNeverRepeatedUnderTheOtherLists(t *testing.T) {
	mgr, issue, _ := issueWithQuestions(t)
	got, err := mgr.ListRulesOfIssueWith(context.Background(), "owner", issue.ID, mwanachamataskmanager.RuleFilter{Statuses: []string{"open", "answered", "dropped"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Linked) != 0 || len(got.Mentioned) != 0 || len(got.Suggested) != 0 {
		t.Fatalf("linked %d, mentioned %d, suggested %d, want none", len(got.Linked), len(got.Mentioned), len(got.Suggested))
	}
}

func TestIssueQuestions_AStatusThatIsNotAQuestionsIsRefused(t *testing.T) {
	mgr, issue, _ := issueWithQuestions(t)
	for _, status := range []string{"bogus", "in_force"} {
		_, err := mgr.ListRulesOfIssueWith(context.Background(), "owner", issue.ID, mwanachamataskmanager.RuleFilter{Statuses: []string{status}})
		if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("status %q: err %v, want ErrInvalidRule", status, err)
		}
	}
}

func TestIssueQuestions_TheOldReadStillAnswersTheOpenOnes(t *testing.T) {
	mgr, issue, asked := issueWithQuestions(t)
	got, err := mgr.ListRulesOfIssue(context.Background(), "owner", issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if codes := questionCodes(got); len(codes) != 1 || codes[0] != asked["open"].Code {
		t.Fatalf("questions %v", codes)
	}
}
