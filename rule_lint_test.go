package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type goneAre map[string]bool

func (g goneAre) PathExists(_ context.Context, _, path string) (bool, error) {
	return !g[path], nil
}

func lintRuleFindings(t *testing.T, mgr mwanachamataskmanager.TaskManager) []mwanachamataskmanager.LintRuleItem {
	t.Helper()
	out, err := mgr.Lint(context.Background(), mwanachamataskmanager.LintFilter{RequesterID: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	return out.Rules
}

func linkFile(t *testing.T, mgr mwanachamataskmanager.TaskManager, code, path string) {
	t.Helper()
	if _, err := mgr.AddRuleFile(context.Background(), "owner", code, "repo-a", path, 0, ""); err != nil {
		t.Fatal(err)
	}
}

func TestRuleLint_AnInForceRuleWithAMissingFileIsReported(t *testing.T) {
	mgr := mwanachamataskmanager.WithRepoPathChecker(newTestManager(t), goneAre{"gone.go": true})
	bad := ruleInForceWhen(t, mgr, "Points at a gone file", "")
	good := ruleInForceWhen(t, mgr, "Points at a live file", "")
	linkFile(t, mgr, bad.Code, "gone.go")
	linkFile(t, mgr, good.Code, "live.go")

	got := lintRuleFindings(t, mgr)

	if len(got) != 1 || got[0].Code != bad.Code {
		t.Fatalf("findings %+v", got)
	}
	if len(got[0].Findings) != 1 || got[0].Findings[0].Rule != "rule_file_missing" {
		t.Fatalf("findings %+v", got[0].Findings)
	}
}

func TestRuleLint_OneFindingPerMissingLink(t *testing.T) {
	mgr := mwanachamataskmanager.WithRepoPathChecker(newTestManager(t), goneAre{"a.go": true, "b.go": true})
	rule := ruleInForceWhen(t, mgr, "Two gone files", "")
	linkFile(t, mgr, rule.Code, "a.go")
	linkFile(t, mgr, rule.Code, "b.go")

	got := lintRuleFindings(t, mgr)

	if len(got) != 1 || len(got[0].Findings) != 2 {
		t.Fatalf("findings %+v", got)
	}
}

func TestRuleLint_NothingIsReportedWithoutACheckerOrForARuleNotInForce(t *testing.T) {
	ctx := context.Background()
	plain := newTestManager(t)
	rule := ruleInForceWhen(t, plain, "No checker", "")
	linkFile(t, plain, rule.Code, "gone.go")
	if got := lintRuleFindings(t, plain); len(got) != 0 {
		t.Fatalf("without a checker %+v", got)
	}

	mgr := mwanachamataskmanager.WithRepoPathChecker(newTestManager(t), goneAre{"gone.go": true})
	retired := ruleInForceWhen(t, mgr, "Retired", "")
	linkFile(t, mgr, retired.Code, "gone.go")
	if _, err := mgr.SetRuleStatus(ctx, "owner", retired.Code, "retired"); err != nil {
		t.Fatal(err)
	}
	if got := lintRuleFindings(t, mgr); len(got) != 0 {
		t.Fatalf("retired rule %+v", got)
	}
}
