package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func (m *taskManager) saveRuleFrom(ctx context.Context, db *gorm.DB, rule RuleRecord, readStatus string) error {
	row, err := m.store.Row(roleRule, rule)
	if err != nil {
		return err
	}
	res := db.WithContext(ctx).Table(m.store.Table(roleRule)).
		Where("id = ? AND status = ?", rule.ID, readStatus).Updates(row)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrRuleChanged
	}
	return nil
}

func (m *taskManager) UpdateRule(ctx context.Context, requesterID, code string, patch RulePatch) (RuleRecord, error) {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return RuleRecord{}, err
	}
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleRecord{}, err
	}
	return m.updateRead(ctx, rule, patch)
}

func (m *taskManager) updateRead(ctx context.Context, rule RuleRecord, patch RulePatch) (RuleRecord, error) {
	readStatus := rule.Status
	if readStatus == ruleSuperseded || readStatus == ruleRetired {
		return RuleRecord{}, fmt.Errorf("%w: %s is %s and its words stay as they are", ErrInvalidRule, rule.Code, readStatus)
	}
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return RuleRecord{}, errNeedsKindAndTitle
		}
		rule.Title = title
	}
	if patch.Body != nil {
		rule.Body = *patch.Body
	}
	if patch.Tags != nil {
		tags, err := m.cleanAndRegisterTags(ctx, *patch.Tags)
		if err != nil {
			return RuleRecord{}, fmt.Errorf("UpdateRule: %w", err)
		}
		rule.Tags = tags
	}
	if patch.AppliesWhen != nil {
		appliesWhen, err := appliesWhenOfKind(rule.Kind, *patch.AppliesWhen)
		if err != nil {
			return RuleRecord{}, err
		}
		rule.AppliesWhen = appliesWhen
	}
	if patch.DecidedBy != nil {
		rule.DecidedBy = *patch.DecidedBy
	}
	if patch.DecidedAt != nil {
		rule.DecidedAt = *patch.DecidedAt
	}
	if patch.Priority != nil {
		if rule.Kind != ruleKindQuestion {
			return RuleRecord{}, errPriorityOfKind(rule.Kind)
		}
		if !questionPriorityKnown(*patch.Priority) {
			return RuleRecord{}, errPriority(*patch.Priority)
		}
		rule.Priority = *patch.Priority
	}
	rule.UpdatedAt = nowStamp()
	if err := m.saveRuleFrom(ctx, m.db, rule, readStatus); err != nil {
		return RuleRecord{}, fmt.Errorf("UpdateRule: %w", err)
	}
	return rule, nil
}

func (m *taskManager) SetRuleStatus(ctx context.Context, requesterID, code, status string) (RuleRecord, error) {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return RuleRecord{}, err
	}
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleRecord{}, err
	}
	return m.setStatusRead(ctx, rule, status)
}

func (m *taskManager) setStatusRead(ctx context.Context, rule RuleRecord, status string) (RuleRecord, error) {
	readStatus := rule.Status
	if readStatus == ruleSuperseded {
		return RuleRecord{}, fmt.Errorf("%w: %s is superseded by %s and stays as it is", ErrInvalidRule, rule.Code, rule.SupersededBy)
	}
	if status == ruleSuperseded {
		return RuleRecord{}, fmt.Errorf("%w: a record becomes superseded by superseding it with a replacement", ErrInvalidRule)
	}
	if !ruleStatusAllowed(rule.Kind, status) {
		return RuleRecord{}, errStatusOfKind(rule.Kind, status)
	}
	if status == questionAnswered {
		return RuleRecord{}, fmt.Errorf("%w: a question is answered by naming the decision that settles it", ErrInvalidRule)
	}
	rule.Status = status
	if status == questionOpen || status == questionDropped {
		rule.AnsweredBy = ""
	}
	rule.UpdatedAt = nowStamp()
	if err := m.saveRuleFrom(ctx, m.db, rule, readStatus); err != nil {
		return RuleRecord{}, fmt.Errorf("SetRuleStatus: %w", err)
	}
	return rule, nil
}

func (m *taskManager) AnswerQuestion(ctx context.Context, requesterID, code, decisionCode string) (RuleRecord, error) {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return RuleRecord{}, err
	}
	if strings.TrimSpace(decisionCode) == "" {
		return RuleRecord{}, fmt.Errorf("%w: answering a question needs the code of the decision that settles it", ErrInvalidRule)
	}
	question, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleRecord{}, err
	}
	if err := checkAnswerable(question); err != nil {
		return RuleRecord{}, err
	}
	decision, err := m.GetRuleByCode(ctx, requesterID, decisionCode)
	if err != nil {
		return RuleRecord{}, err
	}
	return m.answerRead(ctx, question, decision)
}

func checkAnswerable(question RuleRecord) error {
	if question.Kind != ruleKindQuestion {
		return fmt.Errorf("%w: %s is a %s, and only a question is answered", ErrInvalidRule, question.Code, question.Kind)
	}
	if question.Status == questionDropped {
		return fmt.Errorf("%w: %s is dropped; reopen it before answering", ErrInvalidRule, question.Code)
	}
	return nil
}

func (m *taskManager) answerRead(ctx context.Context, question, decision RuleRecord) (RuleRecord, error) {
	if err := checkAnswerable(question); err != nil {
		return RuleRecord{}, err
	}
	if decision.Kind != ruleKindDecision {
		return RuleRecord{}, fmt.Errorf("%w: a question is answered by a decision, and %s is a %s", ErrInvalidRule, decision.Code, decision.Kind)
	}
	readStatus := question.Status
	question.Status, question.AnsweredBy = questionAnswered, decision.Code
	question.UpdatedAt = nowStamp()
	if err := m.saveRuleFrom(ctx, m.db, question, readStatus); err != nil {
		return RuleRecord{}, fmt.Errorf("AnswerQuestion: %w", err)
	}
	return question, nil
}
