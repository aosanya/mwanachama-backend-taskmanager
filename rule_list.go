package mwanachamataskmanager

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func defaultRuleStatuses(kind string) []string {
	switch kind {
	case ruleKindQuestion:
		return []string{questionOpen}
	case ruleKindRule:
		return []string{ruleProposed, ruleInForce}
	case ruleKindDecision:
		return []string{ruleProposed, ruleDecided}
	}
	return []string{ruleProposed, ruleInForce, ruleDecided, questionOpen}
}

func (m *taskManager) ListRules(ctx context.Context, requesterID string, filter RuleFilter) ([]RuleRecord, error) {
	if err := m.requireRuleAccess(ctx, requesterID, false); err != nil {
		return nil, err
	}
	if filter.Kind != "" && !ruleKindKnown(filter.Kind) {
		return nil, ErrInvalidRule
	}
	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = defaultRuleStatuses(filter.Kind)
	}
	for _, status := range statuses {
		known := ruleStatusAllowed(ruleKindRule, status) || ruleStatusAllowed(ruleKindDecision, status) || ruleStatusAllowed(ruleKindQuestion, status)
		if filter.Kind != "" {
			known = ruleStatusAllowed(filter.Kind, status)
		}
		if !known {
			if filter.Kind == "" {
				return nil, fmt.Errorf("%w: unknown status %q", ErrInvalidRule, status)
			}
			return nil, errStatusOfKind(filter.Kind, status)
		}
	}
	out, err := m.rulesCarrying(ctx, statuses, filter.Kind, cleanTags(filter.Tags))
	if err != nil {
		return nil, fmt.Errorf("ListRules: %w", err)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		_, a := splitCode(out[i].Code)
		_, b := splitCode(out[j].Code)
		return a < b
	})
	return out, nil
}

func (m *taskManager) rulesCarrying(ctx context.Context, statuses []string, kind string, tags []string) ([]RuleRecord, error) {
	var out []RuleRecord
	for offset := 0; ; offset += maxListPage {
		q := m.store.Query(ctx, roleRule).Where("status IN ?", statuses)
		if kind != "" {
			q = q.Where("kind = ?", kind)
		}
		page, err := specstore.List[RuleRecord](m.store, q.Order("id").Limit(maxListPage).Offset(offset), roleRule)
		if err != nil {
			return nil, err
		}
		for _, rule := range page {
			if carriesEvery(rule.Tags, tags) {
				out = append(out, rule)
			}
		}
		if len(page) < maxListPage {
			return out, nil
		}
	}
}

func carriesEvery(have, want []string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if strings.EqualFold(h, w) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
