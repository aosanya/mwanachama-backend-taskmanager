package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

const (
	ruleKindRule     = "rule"
	ruleKindDecision = "decision"
	ruleKindQuestion = "question"

	ruleProposed     = "proposed"
	ruleInForce      = "in_force"
	ruleDecided      = "decided"
	ruleSuperseded   = "superseded"
	ruleRetired      = "retired"
	questionOpen     = "open"
	questionAnswered = "answered"
	questionDropped  = "dropped"

	questionDefaultPriority = "P2"
)

var (
	ErrInvalidRule  = errors.New("invalid rule, decision or question")
	ErrRuleNotFound = errors.New("rule not found")
	ErrRuleStale    = errors.New("the record changed since it was read; read it again and send its new updated_at")
	ErrRuleChanged  = fmt.Errorf("%w: the record changed while it was being written; read it and try again", ErrInvalidRule)

	errNeedsKindAndTitle = fmt.Errorf("%w: a rule, decision or question needs a kind (rule, decision or question) and a title", ErrInvalidRule)
)

func cleanTags(tags []string) []string {
	var out []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag != "" && !containsFold(out, tag) {
			out = append(out, tag)
		}
	}
	return out
}

func (m *taskManager) cleanAndRegisterTags(ctx context.Context, tags []string) ([]string, error) {
	cleaned := cleanTags(tags)
	for _, tag := range cleaned {
		if _, err := m.upsertTagByName(ctx, tag); err != nil {
			return nil, err
		}
	}
	return cleaned, nil
}

func ruleCodePrefix(kind string) string {
	switch kind {
	case ruleKindDecision:
		return "D"
	case ruleKindQuestion:
		return "Q"
	}
	return "R"
}

var questionPriorities = []string{"P1", "P2", "P3"}

func questionPriorityKnown(priority string) bool {
	for _, known := range questionPriorities {
		if known == priority {
			return true
		}
	}
	return false
}

func errPriority(priority string) error {
	return fmt.Errorf("%w: a priority is %s, not %q", ErrInvalidRule, strings.Join(questionPriorities, ", "), priority)
}

func errPriorityOfKind(kind string) error {
	return fmt.Errorf("%w: only a question has a priority, not a %s", ErrInvalidRule, kind)
}

func ruleKindKnown(kind string) bool {
	return kind == ruleKindRule || kind == ruleKindDecision || kind == ruleKindQuestion
}

func startingStatus(kind string) string {
	if kind == ruleKindQuestion {
		return questionOpen
	}
	return ruleProposed
}

func settleNewPriority(rule *RuleRecord) error {
	if rule.Kind != ruleKindQuestion {
		if rule.Priority != "" {
			return errPriorityOfKind(rule.Kind)
		}
		return nil
	}
	if rule.Priority == "" {
		rule.Priority = questionDefaultPriority
	}
	if !questionPriorityKnown(rule.Priority) {
		return errPriority(rule.Priority)
	}
	return nil
}

func (m *taskManager) CreateRule(ctx context.Context, requesterID string, rule RuleRecord) (RuleRecord, error) {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return RuleRecord{}, err
	}
	rule.Title = strings.TrimSpace(rule.Title)
	if rule.Title == "" || !ruleKindKnown(rule.Kind) {
		return RuleRecord{}, errNeedsKindAndTitle
	}
	if err := settleNewPriority(&rule); err != nil {
		return RuleRecord{}, err
	}
	if err := settleQuestionShape(&rule); err != nil {
		return RuleRecord{}, err
	}
	rule.AnsweredBy, rule.Answer = "", ""
	appliesWhen, err := appliesWhenOfKind(rule.Kind, rule.AppliesWhen)
	if err != nil {
		return RuleRecord{}, err
	}
	rule.AppliesWhen = appliesWhen
	tags, err := m.cleanAndRegisterTags(ctx, rule.Tags)
	if err != nil {
		return RuleRecord{}, fmt.Errorf("CreateRule: %w", err)
	}
	rule.Tags = tags
	rule.ID = newSpecID()
	rule.Status = startingStatus(rule.Kind)
	rule.Supersedes, rule.SupersededBy = "", ""
	now := nowStamp()
	rule.CreatedAt, rule.UpdatedAt = now, now

	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		code, err := m.nextCode(ctx, tx, "rule_"+rule.Kind, ruleCodePrefix(rule.Kind))
		if err != nil {
			return err
		}
		rule.Code = code
		row, err := m.store.Row(roleRule, rule)
		if err != nil {
			return err
		}
		return tx.Table(m.store.Table(roleRule)).Create(row).Error
	})
	if err != nil {
		return RuleRecord{}, fmt.Errorf("CreateRule: %w", err)
	}
	return rule, nil
}

func (m *taskManager) GetRuleByCode(ctx context.Context, requesterID, code string) (RuleRecord, error) {
	if err := m.requireRuleAccess(ctx, requesterID, false); err != nil {
		return RuleRecord{}, err
	}
	var rule RuleRecord
	q := m.store.Query(ctx, roleRule).Where("code = ?", strings.TrimSpace(code))
	if err := m.store.Take(q, roleRule, &rule, ErrRuleNotFound); err != nil {
		return RuleRecord{}, err
	}
	return rule, nil
}
