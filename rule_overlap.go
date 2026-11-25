package mwanachamataskmanager

import (
	"context"
	"fmt"
	"sort"
)

const (
	OverlapSharedKey   = "shared_key"
	OverlapSharedIssue = "shared_issue"
)

type RuleOverlap struct {
	First        RuleRecord `json:"first"`
	Second       RuleRecord `json:"second"`
	Reasons      []string   `json:"reasons"`
	SharedKey    string     `json:"shared_key,omitempty"`
	SharedIssues []string   `json:"shared_issues,omitempty"`
}

type RuleOverlapList struct {
	Items            []RuleOverlap `json:"items"`
	HiddenCompatible int           `json:"hidden_compatible"`
}

type conditionedRule struct {
	Rule RuleRecord
	Key  string
	Set  RuleSet
}

func (m *taskManager) conditionedRules(ctx context.Context, requesterID string) ([]conditionedRule, error) {
	inForce, err := m.ListRules(ctx, requesterID, RuleFilter{Kind: ruleKindRule, Statuses: []string{ruleInForce}})
	if err != nil {
		return nil, err
	}
	out := []conditionedRule{}
	for _, rule := range inForce {
		set, ok := appliesWhenSet(rule)
		if !ok {
			continue
		}
		key, err := set.Key()
		if err != nil {
			continue
		}
		out = append(out, conditionedRule{Rule: rule, Key: key, Set: set})
	}
	sort.Slice(out, func(i, j int) bool {
		_, a := splitCode(out[i].Rule.Code)
		_, b := splitCode(out[j].Rule.Code)
		return a < b
	})
	return out, nil
}

func overlapOf(a, b conditionedRule, matched map[string]map[string]bool) (RuleOverlap, bool) {
	overlap := RuleOverlap{First: a.Rule, Second: b.Rule}
	if a.Key == b.Key {
		overlap.Reasons = append(overlap.Reasons, OverlapSharedKey)
		overlap.SharedKey = a.Key
	}
	for code := range matched[a.Rule.ID] {
		if matched[b.Rule.ID][code] {
			overlap.SharedIssues = append(overlap.SharedIssues, code)
		}
	}
	if len(overlap.SharedIssues) > 0 {
		sort.Strings(overlap.SharedIssues)
		overlap.Reasons = append(overlap.Reasons, OverlapSharedIssue)
	}
	return overlap, len(overlap.Reasons) > 0
}

func (m *taskManager) ListRuleOverlaps(ctx context.Context, requesterID string) (RuleOverlapList, error) {
	if err := m.requireRuleAccess(ctx, requesterID, false); err != nil {
		return RuleOverlapList{}, err
	}
	rules, err := m.conditionedRules(ctx, requesterID)
	if err != nil {
		return RuleOverlapList{}, fmt.Errorf("ListRuleOverlaps: %w", err)
	}
	matched, err := m.issuesMatchedBy(ctx, requesterID, rules)
	if err != nil {
		return RuleOverlapList{}, fmt.Errorf("ListRuleOverlaps: %w", err)
	}
	judged, err := m.compatiblePairs(ctx)
	if err != nil {
		return RuleOverlapList{}, fmt.Errorf("ListRuleOverlaps: %w", err)
	}
	out := RuleOverlapList{Items: []RuleOverlap{}}
	for i, a := range rules {
		for _, b := range rules[i+1:] {
			overlap, ok := overlapOf(a, b, matched)
			if !ok {
				continue
			}
			if judged[ruleCompatibleKey(a.Rule.ID, b.Rule.ID)] {
				out.HiddenCompatible++
				continue
			}
			out.Items = append(out.Items, overlap)
		}
	}
	return out, nil
}
