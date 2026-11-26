package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestRuleStamp_SupersedingMovesTheOldRecordsStampPastItsRead(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreateRule(t, mgr, "rule", "No comments")
	mustSetStatus(t, mgr, made.Code, "in_force")
	read := mustReadRule(t, mgr, made.Code)

	if _, err := mgr.SupersedeRule(ctx, "owner", made.Code, mwanachamataskmanager.RuleRecord{Title: "No comments in code"}); err != nil {
		t.Fatal(err)
	}
	if after := mustReadRule(t, mgr, made.Code); after.UpdatedAt <= read.UpdatedAt {
		t.Fatalf("updated_at %q did not move past the read %q", after.UpdatedAt, read.UpdatedAt)
	}
}

func TestRuleStamp_AnsweringMovesTheQuestionsStampSoAnEarlierReadIsStale(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	question := mustCreateQuestion(t, mgr, "Does a bad example sit beside the good one?", "P1")
	decision := mustCreateRule(t, mgr, "decision", "Show both")
	read := mustReadRule(t, mgr, question.Code)

	if _, err := mgr.AnswerQuestion(ctx, "owner", question.Code, decision.Code); err != nil {
		t.Fatal(err)
	}
	priority := "P2"
	_, err := mgr.UpdateRule(ctx, "owner", question.Code, mwanachamataskmanager.RulePatch{Priority: &priority, ExpectedUpdatedAt: &read.UpdatedAt})
	if !errors.Is(err, mwanachamataskmanager.ErrRuleStale) {
		t.Fatalf("err %v, want ErrRuleStale", err)
	}
}

func TestRuleStamp_AStaleWriteToARetiredRuleIsStillABadInputError(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := mustCreateRule(t, mgr, "rule", "No comments")
	mustSetStatus(t, mgr, made.Code, "in_force")
	read := mustReadRule(t, mgr, made.Code)
	mustSetStatus(t, mgr, made.Code, "retired")

	title := "late"
	_, err := mgr.UpdateRule(ctx, "owner", made.Code, mwanachamataskmanager.RulePatch{Title: &title, ExpectedUpdatedAt: &read.UpdatedAt})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) || errors.Is(err, mwanachamataskmanager.ErrRuleStale) {
		t.Fatalf("err %v, want ErrInvalidRule and not ErrRuleStale", err)
	}
}
