package mwanachamataskmanager

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func errQuestionOnly(kind string) error {
	return fmt.Errorf("%w: only a question has a target role and options, not a %s", ErrInvalidRule, kind)
}

func errOption(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidRule}, args...)...)
}

func cleanOptions(options []QuestionOption) ([]QuestionOption, error) {
	var out []QuestionOption
	recommended := 0
	for i, option := range options {
		option.Text = strings.TrimSpace(option.Text)
		option.Reason = strings.TrimSpace(option.Reason)
		if option.Text == "" {
			return nil, errOption("option %d has no text", i)
		}
		if option.Reason != "" && !option.Recommended {
			return nil, errOption("option %d has a reason but is not the recommended one", i)
		}
		if strings.ContainsAny(option.Reason, "\r\n") {
			return nil, errOption("the reason of option %d is more than one line", i)
		}
		if option.Recommended {
			recommended++
			if option.Reason == "" {
				return nil, errOption("the recommended option %d needs a one-line reason", i)
			}
		}
		out = append(out, option)
	}
	if recommended > 1 {
		return nil, errOption("%d options are recommended; at most one may be", recommended)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Recommended && !out[b].Recommended })
	return out, nil
}

func settleQuestionShape(rule *RuleRecord) error {
	rule.TargetRole = strings.TrimSpace(rule.TargetRole)
	if rule.Kind != ruleKindQuestion {
		if rule.TargetRole != "" || len(rule.Options) > 0 {
			return errQuestionOnly(rule.Kind)
		}
		return nil
	}
	if rule.TargetRole == "" {
		return fmt.Errorf("%w: a question needs the agency role that answers it", ErrInvalidRule)
	}
	options, err := cleanOptions(rule.Options)
	if err != nil {
		return err
	}
	rule.Options = options
	return nil
}

func (m *taskManager) requireTargetRole(ctx context.Context, requesterID string, question RuleRecord) error {
	if m.roleHolder == nil || requesterID == "" || question.TargetRole == "" {
		return nil
	}
	holds, err := m.roleHolder(ctx, requesterID, question.TargetRole)
	if err != nil {
		return fmt.Errorf("answering %s: the role: %w", question.Code, err)
	}
	if !holds {
		return fmt.Errorf("%w: answering %s needs the role %s", ErrBacklogForbidden, question.Code, question.TargetRole)
	}
	return nil
}

type answerWay int

const (
	answerNone answerWay = iota
	answerByDecision
	answerByOption
	answerByWords
)

func wayOfAnswering(answer QuestionAnswer) (answerWay, error) {
	var ways []answerWay
	if strings.TrimSpace(answer.DecisionCode) != "" {
		ways = append(ways, answerByDecision)
	}
	if answer.Option != nil {
		ways = append(ways, answerByOption)
	}
	if strings.TrimSpace(answer.Other) != "" {
		ways = append(ways, answerByWords)
	}
	if len(ways) != 1 {
		return answerNone, fmt.Errorf("%w: answer a question with exactly one of decision_code, option or other", ErrInvalidRule)
	}
	return ways[0], nil
}

func answerText(question RuleRecord, way answerWay, answer QuestionAnswer) (string, error) {
	if way == answerByWords {
		return strings.TrimSpace(answer.Other), nil
	}
	index := *answer.Option
	if index < 0 || index >= len(question.Options) {
		return "", fmt.Errorf("%w: %s has %d options, so option %d is not one of them", ErrInvalidRule, question.Code, len(question.Options), index)
	}
	return question.Options[index].Text, nil
}

func (m *taskManager) AnswerQuestionWith(ctx context.Context, requesterID, code string, answer QuestionAnswer) (RuleRecord, error) {
	way, err := wayOfAnswering(answer)
	if err != nil {
		return RuleRecord{}, err
	}
	if err := m.requireRuleAccess(ctx, requesterID, way == answerByDecision); err != nil {
		return RuleRecord{}, err
	}
	question, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleRecord{}, err
	}
	if err := checkAnswerable(question); err != nil {
		return RuleRecord{}, err
	}
	if err := m.requireTargetRole(ctx, requesterID, question); err != nil {
		return RuleRecord{}, err
	}
	if way == answerByDecision {
		decision, err := m.GetRuleByCode(ctx, requesterID, answer.DecisionCode)
		if err != nil {
			return RuleRecord{}, err
		}
		return m.answerRead(ctx, question, decision)
	}
	text, err := answerText(question, way, answer)
	if err != nil {
		return RuleRecord{}, err
	}
	return m.closeWithText(ctx, question, text)
}

func (m *taskManager) closeWithText(ctx context.Context, question RuleRecord, text string) (RuleRecord, error) {
	readStatus := question.Status
	question.Status, question.AnsweredBy, question.Answer = questionAnswered, "", text
	question.UpdatedAt = stampAfter(question.UpdatedAt)
	if err := m.saveRuleFrom(ctx, m.db, question, readStatus); err != nil {
		return RuleRecord{}, fmt.Errorf("AnswerQuestion: %w", err)
	}
	return question, nil
}
