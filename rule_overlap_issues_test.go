package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func issueTagged(t *testing.T, mgr mwanachamataskmanager.TaskManager, title string, tags ...string) mwanachamataskmanager.Task {
	t.Helper()
	issue, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: title, Tags: tags})
	if err != nil {
		t.Fatal(err)
	}
	return issue
}

func TestRuleOverlap_DifferentSetsThatMatchOneIssueAreReported(t *testing.T) {
	mgr := newTestManager(t)
	issue := issueTagged(t, mgr, "Style the page", "css", "app")
	a := ruleInForceWhen(t, mgr, "Use rem", tagWhen("css"))
	b := ruleInForceWhen(t, mgr, "Keep it small", tagWhen("app"))

	got := overlaps(t, mgr)

	if len(got) != 1 || got[0].First.Code != a.Code || got[0].Second.Code != b.Code {
		t.Fatalf("overlaps %+v", got)
	}
	if len(got[0].Reasons) != 1 || got[0].Reasons[0] != mwanachamataskmanager.OverlapSharedIssue {
		t.Fatalf("reasons %+v", got[0].Reasons)
	}
	if len(got[0].SharedIssues) != 1 || got[0].SharedIssues[0] != issue.Code {
		t.Fatalf("shared issues %v, want %s", got[0].SharedIssues, issue.Code)
	}
}

func TestRuleOverlap_SetsThatMatchNoCommonIssueAreNotReported(t *testing.T) {
	mgr := newTestManager(t)
	issueTagged(t, mgr, "Style the page", "css")
	issueTagged(t, mgr, "Wire the api", "api")
	ruleInForceWhen(t, mgr, "Use rem", tagWhen("css"))
	ruleInForceWhen(t, mgr, "Version routes", tagWhen("api"))

	if got := overlaps(t, mgr); len(got) != 0 {
		t.Fatalf("overlaps %+v", got)
	}
}

func TestRuleOverlap_APairSharingKeyAndIssueIsOneEntryWithBothReasons(t *testing.T) {
	mgr := newTestManager(t)
	issueTagged(t, mgr, "Style the page", "css")
	ruleInForceWhen(t, mgr, "Use rem", tagWhen("css"))
	ruleInForceWhen(t, mgr, "Use px", tagWhen("css"))

	got := overlaps(t, mgr)

	if len(got) != 1 || len(got[0].Reasons) != 2 {
		t.Fatalf("overlaps %+v", got)
	}
}

func TestRuleOverlap_ASetOnTheIssueFieldsOtherThanTagsIsEvaluatedByTheEngine(t *testing.T) {
	mgr := newTestManager(t)
	issueTagged(t, mgr, "Fix the css bug")
	ruleInForceWhen(t, mgr, "By title", `{"match":"all","rules":[{"field":"title","operator":"contains","value":"css"}]}`)
	ruleInForceWhen(t, mgr, "By tag", tagWhen("css"))

	if got := overlaps(t, mgr); len(got) != 0 {
		t.Fatalf("an issue with no css tag matched the tag rule: %+v", got)
	}
	issueTagged(t, mgr, "Fix the css bug again", "css")
	if got := overlaps(t, mgr); len(got) != 1 || len(got[0].SharedIssues) != 1 {
		t.Fatalf("overlaps %+v", got)
	}
}

func TestRuleOverlap_ADeletedIssueDoesNotCount(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	issue := issueTagged(t, mgr, "Style the page", "css", "app")
	ruleInForceWhen(t, mgr, "Use rem", tagWhen("css"))
	ruleInForceWhen(t, mgr, "Keep it small", tagWhen("app"))
	if err := mgr.DeleteTask(ctx, issue.ID); err != nil {
		t.Fatal(err)
	}

	if got := overlaps(t, mgr); len(got) != 0 {
		t.Fatalf("overlaps %+v", got)
	}
}
