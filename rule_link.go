package mwanachamataskmanager

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var ruleRelationLabels = []string{RelLabelAppliesTo, RelLabelDecidedIn, RelLabelBrokenBy}

var ruleCodeInText = regexp.MustCompile(`\b[RDQ]-[0-9]{1,9}\b`)

const maxReplacementHops = 20

type RuleLink struct {
	Rule        RuleRecord  `json:"rule"`
	Relation    string      `json:"relation"`
	Replacement *RuleRecord `json:"replacement,omitempty"`
}

type IssueRules struct {
	Linked    []RuleLink   `json:"linked"`
	Suggested []RuleRecord `json:"suggested"`
	Mentioned []RuleRecord `json:"mentioned"`
	Questions []RuleRecord `json:"questions"`
}

type RuleIssue struct {
	IssueID   string `json:"issue_id"`
	IssueCode string `json:"issue_code"`
	Title     string `json:"title"`
	Relation  string `json:"relation"`
}

func ruleRelationLabel(relation string) (string, error) {
	for _, label := range ruleRelationLabels {
		if label == relation {
			return label, nil
		}
	}
	return "", fmt.Errorf("%w: a rule link is %s, not %q", ErrInvalidRelationship, strings.Join(ruleRelationLabels, ", "), relation)
}

func (m *taskManager) ruleByID(ctx context.Context, id string) (RuleRecord, error) {
	var rule RuleRecord
	q := m.store.Query(ctx, roleRule).Where("id = ?", id)
	if err := m.store.Take(q, roleRule, &rule, ErrRuleNotFound); err != nil {
		return RuleRecord{}, err
	}
	return rule, nil
}

func (m *taskManager) ruleAndIssue(ctx context.Context, requesterID, code, issueID, relation string) (RuleRecord, Task, string, error) {
	label, err := ruleRelationLabel(relation)
	if err != nil {
		return RuleRecord{}, Task{}, "", err
	}
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return RuleRecord{}, Task{}, "", err
	}
	issue, err := m.GetTaskFor(ctx, requesterID, issueID)
	if err != nil {
		return RuleRecord{}, Task{}, "", err
	}
	return rule, issue, label, nil
}

func (m *taskManager) LinkRule(ctx context.Context, requesterID, code, issueID, relation string) error {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return err
	}
	rule, issue, label, err := m.ruleAndIssue(ctx, requesterID, code, issueID, relation)
	if err != nil {
		return err
	}
	_, err = m.CreateRelationship(ctx, Relationship{Label: label, FromID: rule.ID, ToID: issue.ID})
	return err
}

func (m *taskManager) UnlinkRule(ctx context.Context, requesterID, code, issueID, relation string) error {
	if err := m.requireRuleAccess(ctx, requesterID, true); err != nil {
		return err
	}
	rule, issue, label, err := m.ruleAndIssue(ctx, requesterID, code, issueID, relation)
	if err != nil {
		return err
	}
	return m.DeleteRelationship(ctx, rule.ID, issue.ID, label)
}

func (m *taskManager) replacementOf(ctx context.Context, rule RuleRecord) (*RuleRecord, error) {
	current := rule
	for hops := 0; current.Status == ruleSuperseded && current.SupersededBy != "" && hops < maxReplacementHops; hops++ {
		next, err := m.GetRuleByCode(ctx, "", current.SupersededBy)
		if err != nil {
			return nil, err
		}
		current = next
	}
	if current.ID == rule.ID {
		return nil, nil
	}
	return &current, nil
}

func (m *taskManager) linkedRulesOf(ctx context.Context, issueID string) ([]RuleLink, error) {
	out := []RuleLink{}
	for _, label := range ruleRelationLabels {
		edges, err := m.TraverseRelationships(ctx, issueID, label, DirectionInbound)
		if err != nil {
			return nil, err
		}
		for _, edge := range edges {
			rule, err := m.ruleByID(ctx, edge.FromID)
			if err != nil {
				return nil, err
			}
			if rule.Kind == ruleKindQuestion {
				continue
			}
			replacement, err := m.replacementOf(ctx, rule)
			if err != nil {
				return nil, err
			}
			out = append(out, RuleLink{Rule: rule, Relation: label, Replacement: replacement})
		}
	}
	return out, nil
}

func (m *taskManager) ListIssuesOfRule(ctx context.Context, requesterID, code string) ([]RuleIssue, error) {
	rule, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return nil, err
	}
	out := []RuleIssue{}
	for _, label := range ruleRelationLabels {
		edges, err := m.TraverseRelationships(ctx, rule.ID, label, DirectionOutbound)
		if err != nil {
			return nil, err
		}
		for _, edge := range edges {
			issue, err := m.GetTaskFor(ctx, requesterID, edge.ToID)
			if err != nil {
				continue
			}
			out = append(out, RuleIssue{IssueID: issue.ID, IssueCode: issue.Code, Title: issue.Title, Relation: label})
		}
	}
	return out, nil
}

func sharesATag(have, want []string) bool {
	for _, tag := range want {
		if containsFold(have, tag) {
			return true
		}
	}
	return false
}

func (m *taskManager) suggestedRulesFor(ctx context.Context, requesterID string, issue Task, linked []RuleLink) ([]RuleRecord, error) {
	active, err := m.ListRules(ctx, requesterID, RuleFilter{Statuses: []string{ruleInForce, ruleDecided}})
	if err != nil {
		return nil, err
	}
	taken := map[string]bool{}
	for _, link := range linked {
		taken[link.Rule.ID] = true
	}
	out := []RuleRecord{}
	for _, rule := range active {
		if rule.Kind != ruleKindQuestion && !taken[rule.ID] && sharesATag(issue.Tags, rule.Tags) {
			out = append(out, rule)
		}
	}
	return out, nil
}

func (m *taskManager) openQuestionsFor(ctx context.Context, requesterID string, issue Task) ([]RuleRecord, error) {
	open, err := m.ListRules(ctx, requesterID, RuleFilter{Kind: ruleKindQuestion, Statuses: []string{questionOpen}})
	if err != nil {
		return nil, err
	}
	linked := map[string]bool{}
	for _, label := range ruleRelationLabels {
		edges, err := m.TraverseRelationships(ctx, issue.ID, label, DirectionInbound)
		if err != nil {
			return nil, err
		}
		for _, edge := range edges {
			linked[edge.FromID] = true
		}
	}
	out := []RuleRecord{}
	for _, question := range open {
		if linked[question.ID] || sharesATag(issue.Tags, question.Tags) {
			out = append(out, question)
		}
	}
	return out, nil
}

func (m *taskManager) mentionedRulesFor(ctx context.Context, requesterID string, issue Task, linked []RuleLink, questions []RuleRecord) ([]RuleRecord, error) {
	todos, err := m.issueTodoList(ctx, issue.ID)
	if err != nil {
		return nil, err
	}
	text := []string{issue.Title, issue.Description}
	for _, todo := range todos.Items {
		text = append(text, todo.Title, todo.Description, todo.Instructions)
	}
	seen := map[string]bool{}
	for _, link := range linked {
		seen[link.Rule.Code] = true
	}
	for _, question := range questions {
		seen[question.Code] = true
	}
	out := []RuleRecord{}
	for _, code := range ruleCodeInText.FindAllString(strings.Join(text, "\n"), -1) {
		if seen[code] {
			continue
		}
		seen[code] = true
		rule, err := m.GetRuleByCode(ctx, requesterID, code)
		if err != nil {
			continue
		}
		replacement, err := m.replacementOf(ctx, rule)
		if err != nil {
			return nil, err
		}
		if replacement != nil {
			rule = *replacement
			if seen[rule.Code] {
				continue
			}
			seen[rule.Code] = true
		}
		out = append(out, rule)
	}
	return out, nil
}

func (m *taskManager) ListRulesOfIssue(ctx context.Context, requesterID, issueID string) (IssueRules, error) {
	issue, err := m.GetTaskFor(ctx, requesterID, issueID)
	if err != nil {
		return IssueRules{}, err
	}
	linked, err := m.linkedRulesOf(ctx, issue.ID)
	if err != nil {
		return IssueRules{}, err
	}
	suggested, err := m.suggestedRulesFor(ctx, requesterID, issue, linked)
	if err != nil {
		return IssueRules{}, err
	}
	questions, err := m.openQuestionsFor(ctx, requesterID, issue)
	if err != nil {
		return IssueRules{}, err
	}
	mentioned, err := m.mentionedRulesFor(ctx, requesterID, issue, linked, questions)
	if err != nil {
		return IssueRules{}, err
	}
	return IssueRules{Linked: linked, Suggested: suggested, Mentioned: mentioned, Questions: questions}, nil
}

func (m *taskManager) ListRulesOfDecision(ctx context.Context, requesterID, code string) ([]RuleRecord, error) {
	decision, err := m.GetRuleByCode(ctx, requesterID, code)
	if err != nil {
		return nil, err
	}
	if decision.Kind != ruleKindDecision {
		return nil, fmt.Errorf("%w: only a decision produces rules, and %s is a %s", ErrInvalidRule, decision.Code, decision.Kind)
	}
	settled, err := m.TraverseRelationships(ctx, decision.ID, RelLabelDecidedIn, DirectionOutbound)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []RuleRecord{}
	for _, edge := range settled {
		linked, err := m.TraverseRelationships(ctx, edge.ToID, RelLabelDecidedIn, DirectionInbound)
		if err != nil {
			return nil, err
		}
		for _, back := range linked {
			if seen[back.FromID] {
				continue
			}
			seen[back.FromID] = true
			rule, err := m.ruleByID(ctx, back.FromID)
			if err != nil {
				return nil, err
			}
			if rule.Kind == ruleKindRule {
				out = append(out, rule)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		_, a := splitCode(out[i].Code)
		_, b := splitCode(out[j].Code)
		return a < b
	})
	return out, nil
}
