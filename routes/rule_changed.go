package routes

import (
	"context"
	"errors"

	"github.com/aosanya/mwanachama-backend-shared/dispatch"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type ruleChangedError struct{ cause error }

func (e ruleChangedError) Error() string { return e.cause.Error() }

func (e ruleChangedError) Unwrap() error { return e.cause }

var ruleChanged = dispatch.ByType(ruleChangedError{}, nil)

func markRuleChanged(err error) error {
	if errors.Is(err, mwanachamataskmanager.ErrRuleChanged) {
		return ruleChangedError{cause: err}
	}
	return err
}

type ruleWritesMarked struct {
	mwanachamataskmanager.TaskManager
}

func (m ruleWritesMarked) UpdateRule(ctx context.Context, requesterID, code string, patch mwanachamataskmanager.RulePatch) (mwanachamataskmanager.RuleRecord, error) {
	rule, err := m.TaskManager.UpdateRule(ctx, requesterID, code, patch)
	return rule, markRuleChanged(err)
}

func (m ruleWritesMarked) SetRuleStatus(ctx context.Context, requesterID, code, status string) (mwanachamataskmanager.RuleRecord, error) {
	rule, err := m.TaskManager.SetRuleStatus(ctx, requesterID, code, status)
	return rule, markRuleChanged(err)
}

func (m ruleWritesMarked) SupersedeRule(ctx context.Context, requesterID, code string, replacement mwanachamataskmanager.RuleRecord) (mwanachamataskmanager.RuleRecord, error) {
	rule, err := m.TaskManager.SupersedeRule(ctx, requesterID, code, replacement)
	return rule, markRuleChanged(err)
}

func (m ruleWritesMarked) AnswerQuestionWith(ctx context.Context, requesterID, code string, answer mwanachamataskmanager.QuestionAnswer) (mwanachamataskmanager.RuleRecord, error) {
	rule, err := m.TaskManager.AnswerQuestionWith(ctx, requesterID, code, answer)
	return rule, markRuleChanged(err)
}

func (m ruleWritesMarked) AnswerQuestion(ctx context.Context, requesterID, code, decisionCode string) (mwanachamataskmanager.RuleRecord, error) {
	rule, err := m.TaskManager.AnswerQuestion(ctx, requesterID, code, decisionCode)
	return rule, markRuleChanged(err)
}

func withRuleChangedMarked(tm mwanachamataskmanager.TaskManager) any {
	if tm == nil {
		return nil
	}
	return ruleWritesMarked{tm}
}
