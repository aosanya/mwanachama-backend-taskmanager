package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
)

const lintRuleFileMissing = "rule_file_missing"

type LintRuleItem struct {
	Code     string        `json:"code"`
	Title    string        `json:"title"`
	Findings []LintFinding `json:"findings"`
}

func (m *taskManager) ruleLintItems(ctx context.Context, requesterID string) ([]LintRuleItem, error) {
	items := []LintRuleItem{}
	if m.pathChecker == nil {
		return items, nil
	}
	inForce, err := m.ListRules(ctx, requesterID, RuleFilter{Kind: ruleKindRule, Statuses: []string{ruleInForce}})
	if errors.Is(err, ErrBacklogForbidden) {
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	for _, rule := range inForce {
		links, err := m.ListRuleFiles(ctx, requesterID, rule.Code)
		if err != nil {
			return nil, err
		}
		findings := []LintFinding{}
		for _, link := range links {
			if link.Missing {
				findings = append(findings, LintFinding{
					Rule:    lintRuleFileMissing,
					Message: fmt.Sprintf("%s in %s is linked as an example but is not on main.", link.Path, link.Repo),
				})
			}
		}
		if len(findings) > 0 {
			items = append(items, LintRuleItem{Code: rule.Code, Title: rule.Title, Findings: findings})
		}
	}
	return items, nil
}
