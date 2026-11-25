package mwanachamataskmanager

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func (m *taskManager) SupersedeRule(ctx context.Context, requesterID, code string, replacement RuleRecord) (RuleRecord, error) {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return RuleRecord{}, err
	}
	old, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleRecord{}, err
	}
	return m.supersedeRead(ctx, old, replacement)
}

func (m *taskManager) supersedeRead(ctx context.Context, old, replacement RuleRecord) (RuleRecord, error) {
	if old.Kind == ruleKindQuestion {
		return RuleRecord{}, fmt.Errorf("%w: a question is answered or dropped, never superseded", ErrInvalidRule)
	}
	if old.Status != ruleActive[old.Kind] {
		return RuleRecord{}, fmt.Errorf("%w: only a record that is %s can be superseded, and %s is %s", ErrInvalidRule, ruleActive[old.Kind], old.Code, old.Status)
	}
	replacement.Kind = old.Kind
	replacement.Title = strings.TrimSpace(replacement.Title)
	if replacement.Title == "" {
		return RuleRecord{}, errNeedsKindAndTitle
	}
	if err := settleNewPriority(&replacement); err != nil {
		return RuleRecord{}, err
	}
	replacement.AnsweredBy = ""
	if appliesWhenOmitted(replacement.AppliesWhen) {
		replacement.AppliesWhen = old.AppliesWhen
	}
	appliesWhen, err := appliesWhenOfKind(old.Kind, replacement.AppliesWhen)
	if err != nil {
		return RuleRecord{}, err
	}
	replacement.AppliesWhen = appliesWhen
	tags, err := m.cleanAndRegisterTags(ctx, replacement.Tags)
	if err != nil {
		return RuleRecord{}, fmt.Errorf("SupersedeRule: %w", err)
	}
	replacement.Tags = tags
	replacement.ID = newSpecID()
	replacement.Status = ruleActive[old.Kind]
	replacement.Supersedes, replacement.SupersededBy = old.Code, ""
	now := nowStamp()
	replacement.CreatedAt, replacement.UpdatedAt = now, now
	readStatus := old.Status

	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		next, err := m.nextCode(ctx, tx, "rule_"+replacement.Kind, ruleCodePrefix(replacement.Kind))
		if err != nil {
			return err
		}
		replacement.Code = next
		row, err := m.store.Row(roleRule, replacement)
		if err != nil {
			return err
		}
		if err := tx.Table(m.store.Table(roleRule)).Create(row).Error; err != nil {
			return err
		}
		old.Status, old.SupersededBy, old.UpdatedAt = ruleSuperseded, next, stampAfter(old.UpdatedAt)
		return m.saveRuleFrom(ctx, tx, old, readStatus)
	})
	if err != nil {
		return RuleRecord{}, fmt.Errorf("SupersedeRule: %w", err)
	}
	return replacement, nil
}
