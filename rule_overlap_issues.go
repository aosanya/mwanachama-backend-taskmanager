package mwanachamataskmanager

import (
	"context"
	"slices"
	"time"
)

func (m *taskManager) issuesMatchedBy(ctx context.Context, requesterID string, rules []conditionedRule) (map[string]map[string]bool, error) {
	matched := map[string]map[string]bool{}
	if len(rules) < 2 {
		return matched, nil
	}
	needsWaits := false
	for _, rule := range rules {
		needsWaits = needsWaits || hasRuleOn(rule.Set, RuleFieldWaitsOn)
	}
	subjects, err := m.visibleRuleSubjects(ctx, requesterID, needsWaits)
	if err != nil {
		return nil, err
	}
	open := subjects[:0:0]
	for _, subject := range subjects {
		if !slices.Contains(closedStatuses, string(subject.Task.Status)) {
			open = append(open, subject)
		}
	}
	now := time.Now().UTC()
	for _, rule := range rules {
		codes := map[string]bool{}
		for _, subject := range open {
			ok, err := rule.Set.Matches(subject, now)
			if err == nil && ok {
				codes[subject.Task.Code] = true
			}
		}
		matched[rule.Rule.ID] = codes
	}
	return matched, nil
}
