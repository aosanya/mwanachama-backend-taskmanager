package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func earlierAnswers(t *testing.T, mgr mwanachamataskmanager.TaskManager, code string) []string {
	t.Helper()
	notes, err := mgr.ListRuleNotes(context.Background(), mwanachamataskmanager.RuleNoteFilter{RequesterID: "owner", Rule: code})
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, note := range notes {
		texts = append(texts, note.Text)
	}
	return texts
}

func TestReanswer_TheNewAnswerReplacesTheOldAndTheOldIsKeptInTheLog(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	q := askedWithOptions(t, mgr)
	if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, optionAt(0)); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, mwanachamataskmanager.QuestionAnswer{Other: "Neither"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "answered" || got.Answer != "Neither" {
		t.Fatalf("got %+v", got)
	}
	texts := earlierAnswers(t, mgr, q.Code)
	if len(texts) != 1 || texts[0] != "Earlier answer: Spaces" {
		t.Fatalf("log %v, want one earlier answer", texts)
	}
}

func TestReanswer_EachEarlierAnswerGetsItsOwnNote(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	q := askedWithOptions(t, mgr)
	for _, other := range []string{"First", "Second", "Third"} {
		if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, mwanachamataskmanager.QuestionAnswer{Other: other}); err != nil {
			t.Fatal(err)
		}
	}
	if texts := earlierAnswers(t, mgr, q.Code); len(texts) != 2 {
		t.Fatalf("log %v, want two earlier answers", texts)
	}
}

func TestReanswer_TheSameWordsAgainWriteNoNote(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	q := askedWithOptions(t, mgr)
	for i := 0; i < 2; i++ {
		if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, optionAt(0)); err != nil {
			t.Fatal(err)
		}
	}
	if texts := earlierAnswers(t, mgr, q.Code); len(texts) != 0 {
		t.Fatalf("log %v, want none", texts)
	}
}

func TestReanswer_ADecisionThatSettledItIsKeptByItsCode(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	q := askedWithOptions(t, mgr)
	decision := mustCreateRule(t, mgr, "decision", "Use spaces")
	if _, err := mgr.AnswerQuestion(ctx, "owner", q.Code, decision.Code); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, optionAt(2)); err != nil {
		t.Fatal(err)
	}
	texts := earlierAnswers(t, mgr, q.Code)
	if len(texts) != 1 || texts[0] != "Earlier answer: settled by "+decision.Code {
		t.Fatalf("log %v", texts)
	}
}

func TestReanswer_ADroppedQuestionIsStillRefused(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	q := askedWithOptions(t, mgr)
	if _, err := mgr.SetRuleStatus(ctx, "owner", q.Code, "dropped"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AnswerQuestionWith(ctx, "owner", q.Code, optionAt(0)); err == nil {
		t.Fatal("a dropped question was answered")
	}
}
