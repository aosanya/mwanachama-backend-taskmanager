package mwanachamataskmanager

import (
	"fmt"
	"strings"
)

var ruleStatuses = map[string][]string{
	ruleKindRule:     {ruleProposed, ruleInForce, ruleSuperseded, ruleRetired},
	ruleKindDecision: {ruleProposed, ruleDecided, ruleSuperseded},
	ruleKindQuestion: {questionOpen, questionAnswered, questionDropped},
}

var ruleActive = map[string]string{
	ruleKindRule:     ruleInForce,
	ruleKindDecision: ruleDecided,
}

func ruleStatusAllowed(kind, status string) bool {
	for _, s := range ruleStatuses[kind] {
		if s == status {
			return true
		}
	}
	return false
}

func errStatusOfKind(kind, status string) error {
	return fmt.Errorf("%w: a %s is %s, not %q", ErrInvalidRule, kind, strings.Join(ruleStatuses[kind], ", "), status)
}
