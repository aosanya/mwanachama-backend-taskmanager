package mwanachamataskmanager

import "testing"

func wellFiledSubject() lintSubject {
	return lintSubject{
		Task: Task{
			Description: "Makes the thing.\nDone looks like: the tests pass.",
			Files:       FileList{"code/a.go"},
			Effort:      TaskEffortLow,
			Priority:    TaskPriorityHigh,
		},
		Todos: []TaskTodo{
			{Title: "Build it", Description: "what, where, when"},
			{Title: "Documentation: one row", Description: "a few lines"},
		},
	}
}

func TestLintRulesAreCleanForAWellFiledSubject(t *testing.T) {
	if findings := findingsOf(wellFiledSubject()); len(findings) != 0 {
		t.Errorf("findings %v, want none", findings)
	}
}

func TestLintNoPriorityFiresOnAnIssueWithoutOne(t *testing.T) {
	subject := wellFiledSubject()
	subject.Task.Priority = ""

	findings := findingsOf(subject)

	if len(findings) != 1 || findings[0].Rule != "no_priority" {
		t.Errorf("findings %v, want only no_priority", findings)
	}
}

func TestLintRuleNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, rule := range lintRules {
		if seen[rule.Name] {
			t.Errorf("rule %q is declared twice", rule.Name)
		}
		seen[rule.Name] = true
	}
}
