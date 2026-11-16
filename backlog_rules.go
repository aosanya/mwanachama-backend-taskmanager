package mwanachamataskmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidRules = errors.New("invalid backlog rules")

const (
	RuleFieldTag         = "tag"
	RuleFieldMark        = "mark"
	RuleFieldBacklog     = "backlog"
	RuleFieldStep        = "step"
	RuleFieldPriority    = "priority"
	RuleFieldTitle       = "title"
	RuleFieldDescription = "description"
	RuleFieldWaitsOn     = "waits_on"
	RuleFieldLastChanged = "last_changed"
)

const (
	RuleOpHas            = "has"
	RuleOpDoesNotHave    = "does_not_have"
	RuleOpIs             = "is"
	RuleOpIsNot          = "is_not"
	RuleOpContains       = "contains"
	RuleOpDoesNotContain = "does_not_contain"
	RuleOpHasSomething   = "has_something"
	RuleOpHasNothing     = "has_nothing"
	RuleOpInTheLast      = "in_the_last"
	RuleOpNotInTheLast   = "not_in_the_last"
)

const (
	RuleMatchAll = "all"
	RuleMatchAny = "any"
)

var ruleFieldOperators = map[string][]string{
	RuleFieldTag:         {RuleOpHas, RuleOpDoesNotHave},
	RuleFieldMark:        {RuleOpIs, RuleOpIsNot},
	RuleFieldBacklog:     {RuleOpIs, RuleOpIsNot},
	RuleFieldStep:        {RuleOpIs, RuleOpIsNot},
	RuleFieldPriority:    {RuleOpIs, RuleOpIsNot},
	RuleFieldTitle:       {RuleOpContains, RuleOpDoesNotContain},
	RuleFieldDescription: {RuleOpContains, RuleOpDoesNotContain},
	RuleFieldWaitsOn:     {RuleOpHasSomething, RuleOpHasNothing},
	RuleFieldLastChanged: {RuleOpInTheLast, RuleOpNotInTheLast},
}

var ruleFieldOrder = []string{
	RuleFieldTag, RuleFieldMark, RuleFieldBacklog, RuleFieldStep, RuleFieldPriority,
	RuleFieldTitle, RuleFieldDescription, RuleFieldWaitsOn, RuleFieldLastChanged,
}

type Rule struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    string `json:"value,omitempty"`
}

type RuleSet struct {
	Match string `json:"match"`
	Rules []Rule `json:"rules"`
}

type RuleSubject struct {
	Task        Task
	BacklogName string
	StepName    string
	WaitsOn     int
}

func ruleFieldNames() string {
	return strings.Join(ruleFieldOrder, ", ")
}

func canonicalToken(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), " ", "_")
}

func operatorAllowed(field, op string) bool {
	for _, allowed := range ruleFieldOperators[field] {
		if allowed == op {
			return true
		}
	}
	return false
}

func parseSpan(value string) (int, string, error) {
	parts := strings.Fields(strings.ToLower(value))
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("a span is a number and a unit such as \"30 days\", got %q", value)
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil || n <= 0 {
		return 0, "", fmt.Errorf("a span starts with a whole number above zero, got %q", parts[0])
	}
	unit := strings.TrimSuffix(parts[1], "s")
	if unit != "day" && unit != "week" && unit != "month" {
		return 0, "", fmt.Errorf("a span unit is days, weeks or months, got %q", parts[1])
	}
	return n, unit, nil
}

func formatSpan(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func canonicalValue(field, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch field {
	case RuleFieldWaitsOn:
		if value != "" {
			return "", fmt.Errorf("%s takes no value, got %q", field, value)
		}
		return "", nil
	case RuleFieldMark:
		for _, mark := range []string{tagReady, tagWaiting, tagSinbin, tagBlocked} {
			if strings.EqualFold(value, mark) {
				return mark, nil
			}
		}
		return "", fmt.Errorf("mark is Ready, Waiting, Sinbin or Blocked, got %q", value)
	case RuleFieldPriority:
		switch TaskPriority(strings.ToLower(value)) {
		case TaskPriorityLow, TaskPriorityMedium, TaskPriorityHigh, TaskPriorityCritical:
			return strings.ToLower(value), nil
		}
		return "", fmt.Errorf("priority is low, medium, high or critical, got %q", value)
	case RuleFieldLastChanged:
		n, unit, err := parseSpan(value)
		if err != nil {
			return "", err
		}
		return formatSpan(n, unit), nil
	}
	if value == "" {
		return "", fmt.Errorf("%s needs a value", field)
	}
	return strings.ToLower(value), nil
}

func normaliseRule(r Rule) (Rule, error) {
	field := canonicalToken(r.Field)
	if _, ok := ruleFieldOperators[field]; !ok {
		return Rule{}, fmt.Errorf("%w: unknown field %q, the fields are %s", ErrInvalidRules, r.Field, ruleFieldNames())
	}
	op := canonicalToken(r.Operator)
	if !operatorAllowed(field, op) {
		return Rule{}, fmt.Errorf("%w: %s does not take %q, it takes %s", ErrInvalidRules, field, r.Operator, strings.Join(ruleFieldOperators[field], ", "))
	}
	value, err := canonicalValue(field, r.Value)
	if err != nil {
		return Rule{}, fmt.Errorf("%w: %v", ErrInvalidRules, err)
	}
	return Rule{Field: field, Operator: op, Value: value}, nil
}

func fieldRank(field string) int {
	for i, name := range ruleFieldOrder {
		if name == field {
			return i
		}
	}
	return len(ruleFieldOrder)
}

func (rs RuleSet) Normalise() (RuleSet, error) {
	match := canonicalToken(rs.Match)
	if match == "" {
		match = RuleMatchAll
	}
	if match != RuleMatchAll && match != RuleMatchAny {
		return RuleSet{}, fmt.Errorf("%w: match is all or any, got %q", ErrInvalidRules, rs.Match)
	}
	if len(rs.Rules) == 0 {
		return RuleSet{}, fmt.Errorf("%w: a smart backlog needs at least one rule", ErrInvalidRules)
	}
	seen := map[Rule]bool{}
	rules := make([]Rule, 0, len(rs.Rules))
	for _, r := range rs.Rules {
		n, err := normaliseRule(r)
		if err != nil {
			return RuleSet{}, err
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		rules = append(rules, n)
	}
	sort.Slice(rules, func(i, j int) bool {
		a, b := rules[i], rules[j]
		if fieldRank(a.Field) != fieldRank(b.Field) {
			return fieldRank(a.Field) < fieldRank(b.Field)
		}
		if a.Operator != b.Operator {
			return a.Operator < b.Operator
		}
		return a.Value < b.Value
	})
	return RuleSet{Match: match, Rules: rules}, nil
}

func (rs RuleSet) Validate() error {
	_, err := rs.Normalise()
	return err
}

func (rs RuleSet) Key() (string, error) {
	n, err := rs.Normalise()
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(n)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (rs RuleSet) Matches(s RuleSubject, now time.Time) (bool, error) {
	n, err := rs.Normalise()
	if err != nil {
		return false, err
	}
	for _, r := range n.Rules {
		ok := ruleMatches(r, s, now)
		if n.Match == RuleMatchAny && ok {
			return true, nil
		}
		if n.Match == RuleMatchAll && !ok {
			return false, nil
		}
	}
	return n.Match == RuleMatchAll, nil
}

func subjectTags(s RuleSubject) []string {
	tags := append([]string{}, s.Task.Tags...)
	if s.Task.ProjectName != "" {
		tags = append(tags, s.Task.ProjectName)
	}
	return tags
}

func containsFold(list []string, want string) bool {
	for _, item := range list {
		if strings.EqualFold(item, want) {
			return true
		}
	}
	return false
}

func nameOrID(name, id, want string) bool {
	return strings.EqualFold(name, want) || (id != "" && strings.EqualFold(id, want))
}

func spanCutoff(now time.Time, value string) time.Time {
	n, unit, _ := parseSpan(value)
	switch unit {
	case "week":
		return now.AddDate(0, 0, -7*n)
	case "month":
		return now.AddDate(0, -n, 0)
	}
	return now.AddDate(0, 0, -n)
}

func changedWithin(s RuleSubject, value string, now time.Time) bool {
	changed, err := time.Parse(time.RFC3339, s.Task.UpdatedAt)
	if err != nil {
		return false
	}
	return !changed.Before(spanCutoff(now, value))
}

func ruleMatches(r Rule, s RuleSubject, now time.Time) bool {
	switch r.Field {
	case RuleFieldTag:
		has := containsFold(subjectTags(s), r.Value)
		return has == (r.Operator == RuleOpHas)
	case RuleFieldMark:
		is := strings.EqualFold(markOfTags(s.Task.Tags), r.Value)
		return is == (r.Operator == RuleOpIs)
	case RuleFieldBacklog:
		is := nameOrID(s.BacklogName, s.Task.Backlog, r.Value)
		return is == (r.Operator == RuleOpIs)
	case RuleFieldStep:
		is := nameOrID(s.StepName, s.Task.Step, r.Value)
		return is == (r.Operator == RuleOpIs)
	case RuleFieldPriority:
		is := strings.EqualFold(string(s.Task.Priority), r.Value)
		return is == (r.Operator == RuleOpIs)
	case RuleFieldTitle:
		has := strings.Contains(strings.ToLower(s.Task.Title), r.Value)
		return has == (r.Operator == RuleOpContains)
	case RuleFieldDescription:
		has := strings.Contains(strings.ToLower(s.Task.Description), r.Value)
		return has == (r.Operator == RuleOpContains)
	case RuleFieldWaitsOn:
		return (s.WaitsOn > 0) == (r.Operator == RuleOpHasSomething)
	case RuleFieldLastChanged:
		return changedWithin(s, r.Value, now) == (r.Operator == RuleOpInTheLast)
	}
	return false
}
