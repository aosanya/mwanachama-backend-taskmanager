package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func lintRulesFor(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.LintFilter) []mwanachamataskmanager.LintRuleItem {
	t.Helper()
	filter.RequesterID = "owner"
	out, err := mgr.Lint(context.Background(), filter)
	if err != nil {
		t.Fatal(err)
	}
	return out.Rules
}

func TestRuleLint_TheRulesListComesOnlyWithAnUnfilteredFirstPage(t *testing.T) {
	mgr := mwanachamataskmanager.WithRepoPathChecker(newTestManager(t), goneAre{"gone.go": true})
	bad := ruleInForceWhen(t, mgr, "Points at a gone file", "")
	linkFile(t, mgr, bad.Code, "gone.go")
	issue := issueTagged(t, mgr, "An issue")
	issueTagged(t, mgr, "Another issue")

	if got := lintRulesFor(t, mgr, mwanachamataskmanager.LintFilter{}); len(got) != 1 {
		t.Fatalf("unfiltered first page: %+v", got)
	}
	if got := lintRulesFor(t, mgr, mwanachamataskmanager.LintFilter{Limit: 1}); len(got) != 1 {
		t.Fatalf("first page with a limit: %+v", got)
	}
	filtered := map[string]mwanachamataskmanager.LintFilter{
		"issue":   {TaskID: issue.ID},
		"backlog": {Backlog: "backlog"},
		"step":    {Step: "to-do"},
		"page 2":  {Limit: 1, Offset: 1},
	}
	for name, filter := range filtered {
		if got := lintRulesFor(t, mgr, filter); len(got) != 0 {
			t.Fatalf("%s: rules list %+v", name, got)
		}
	}
}
