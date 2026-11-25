package mwanachamataskmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func normalisedAppliesWhen(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}
	var set RuleSet
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&set); err != nil {
		return nil, fmt.Errorf("%w: applies_when is {match, rules}: %v", ErrInvalidRule, err)
	}
	if set.Match == "" && len(set.Rules) == 0 {
		return nil, nil
	}
	normal, err := set.Normalise()
	if err != nil {
		return nil, fmt.Errorf("%w: applies_when: %v", ErrInvalidRule, err)
	}
	out, err := json.Marshal(normal)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func appliesWhenOfKind(kind string, raw json.RawMessage) (json.RawMessage, error) {
	normal, err := normalisedAppliesWhen(raw)
	if err != nil {
		return nil, err
	}
	if len(normal) > 0 && kind != ruleKindRule {
		return nil, fmt.Errorf("%w: only a rule has applies_when, not a %s", ErrInvalidRule, kind)
	}
	return normal, nil
}

func appliesWhenOmitted(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null"))
}

func appliesWhenSet(r RuleRecord) (RuleSet, bool) {
	if len(r.AppliesWhen) == 0 {
		return RuleSet{}, false
	}
	var set RuleSet
	if err := json.Unmarshal(r.AppliesWhen, &set); err != nil {
		return RuleSet{}, false
	}
	return set, true
}
