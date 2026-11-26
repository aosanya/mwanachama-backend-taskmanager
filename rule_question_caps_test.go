package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func nOptions(n int) []mwanachamataskmanager.QuestionOption {
	var out []mwanachamataskmanager.QuestionOption
	for i := 0; i < n; i++ {
		out = append(out, mwanachamataskmanager.QuestionOption{Text: fmt.Sprintf("Option %d", i)})
	}
	return out
}

func TestQuestionCaps_SixOptionsAreKeptAndSevenAreRefused(t *testing.T) {
	mgr := newTestManager(t)
	if _, err := askQuestion(mgr, mwanachamataskmanager.RuleRecord{Title: "Six?", Options: nOptions(6)}); err != nil {
		t.Fatalf("six options: %v", err)
	}
	_, err := askQuestion(mgr, mwanachamataskmanager.RuleRecord{Title: "Seven?", Options: nOptions(7)})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("seven options: err %v, want ErrInvalidRule", err)
	}
}

func TestQuestionCaps_AnOptionIsAtMost120CharactersCountedAsLetters(t *testing.T) {
	mgr := newTestManager(t)
	fits := []mwanachamataskmanager.QuestionOption{{Text: strings.Repeat("é", 120)}}
	if _, err := askQuestion(mgr, mwanachamataskmanager.RuleRecord{Title: "Fits?", Options: fits}); err != nil {
		t.Fatalf("120 letters: %v", err)
	}
	long := []mwanachamataskmanager.QuestionOption{{Text: strings.Repeat("é", 121)}}
	_, err := askQuestion(mgr, mwanachamataskmanager.RuleRecord{Title: "Long?", Options: long})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("121 letters: err %v, want ErrInvalidRule", err)
	}
}

func TestQuestionCaps_TheSameCapsHoldWhenOptionsAreEdited(t *testing.T) {
	mgr := newTestManager(t)
	q := mustAsk(t, mgr, mwanachamataskmanager.RuleRecord{Title: "Edit me"})
	tooMany := nOptions(7)
	_, err := mgr.UpdateRule(context.Background(), "owner", q.Code, mwanachamataskmanager.RulePatch{Options: &tooMany})
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("seven options on update: err %v, want ErrInvalidRule", err)
	}
}
