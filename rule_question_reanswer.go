package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

func earlierAnswerOf(question RuleRecord) string {
	if question.Status != questionAnswered {
		return ""
	}
	if question.AnsweredBy != "" {
		return "settled by " + question.AnsweredBy
	}
	return question.Answer
}

func (m *taskManager) closeWithText(ctx context.Context, requesterID string, question RuleRecord, text string) (RuleRecord, error) {
	return m.saveAnswer(ctx, requesterID, question, "", text)
}

func (m *taskManager) saveAnswer(ctx context.Context, requesterID string, question RuleRecord, decisionCode, text string) (RuleRecord, error) {
	earlier := earlierAnswerOf(question)
	readStatus := question.Status
	answered := text
	if decisionCode != "" {
		answered = "settled by " + decisionCode
	}
	if earlier == answered {
		earlier = ""
	}
	question.Status, question.AnsweredBy, question.Answer = questionAnswered, decisionCode, text
	question.UpdatedAt = stampAfter(question.UpdatedAt)
	err := m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if earlier != "" {
			if err := m.insertEarlierAnswer(tx, requesterID, question, earlier); err != nil {
				return err
			}
		}
		return m.saveRuleFrom(ctx, tx, question, readStatus)
	})
	if err != nil {
		return RuleRecord{}, fmt.Errorf("AnswerQuestion: %w", err)
	}
	return question, nil
}

func (m *taskManager) insertEarlierAnswer(tx *gorm.DB, requesterID string, question RuleRecord, earlier string) error {
	note := RuleNote{
		ID: newSpecID(), RuleID: question.ID, RuleCode: question.Code, Kind: ruleNoteKindNote, Source: noteByPerson,
		Text: "Earlier answer: " + earlier, RelatedCodes: []string{}, Links: []string{}, AuthorID: requesterID,
		CreatedAt: time.Now().UTC().Format(noteTimeForm),
	}
	row, err := m.store.Row(roleRuleNote, note)
	if err != nil {
		return err
	}
	return tx.Table(m.store.Table(roleRuleNote)).Create(row).Error
}

func applyQuestionShape(rule *RuleRecord, patch RulePatch) error {
	if patch.TargetRole == nil && patch.Options == nil {
		return nil
	}
	if rule.Kind != ruleKindQuestion {
		return errQuestionOnly(rule.Kind)
	}
	if patch.TargetRole != nil {
		rule.TargetRole = *patch.TargetRole
	}
	if patch.Options != nil {
		rule.Options = *patch.Options
	}
	return settleQuestionShape(rule)
}
