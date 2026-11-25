package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func linkFixture(t *testing.T) (mwanachamataskmanager.TaskManager, mwanachamataskmanager.RuleRecord, mwanachamataskmanager.Task) {
	t.Helper()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "Work on main")
	issue, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "Ship it"})
	if err != nil {
		t.Fatal(err)
	}
	return mgr, rule, issue
}

func activate(t *testing.T, mgr mwanachamataskmanager.TaskManager, code, status string) {
	t.Helper()
	if _, err := mgr.SetRuleStatus(context.Background(), "owner", code, status); err != nil {
		t.Fatal(err)
	}
}

func TestRuleLinks_ALinkReadsFromBothSides(t *testing.T) {
	ctx := context.Background()
	mgr, rule, issue := linkFixture(t)

	for _, relation := range []string{"applies_to", "decided_in", "broken_by"} {
		if err := mgr.LinkRule(ctx, "owner", rule.Code, issue.ID, relation); err != nil {
			t.Fatalf("%s: %v", relation, err)
		}
	}
	fromIssue, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(fromIssue.Linked) != 3 || fromIssue.Linked[0].Rule.Code != rule.Code {
		t.Fatalf("issue side: %+v", fromIssue.Linked)
	}
	fromRule, err := mgr.ListIssuesOfRule(ctx, "owner", rule.Code)
	if err != nil {
		t.Fatal(err)
	}
	if len(fromRule) != 3 || fromRule[0].IssueID != issue.ID || fromRule[0].IssueCode != issue.Code || fromRule[0].Title != "Ship it" {
		t.Fatalf("rule side: %+v", fromRule)
	}
	relations := map[string]bool{}
	for _, item := range fromRule {
		relations[item.Relation] = true
	}
	if !relations["applies_to"] || !relations["decided_in"] || !relations["broken_by"] {
		t.Fatalf("relations %v", relations)
	}
}

func TestRuleLinks_LinkingTwiceIsOneLinkAndUnlinkRemovesIt(t *testing.T) {
	ctx := context.Background()
	mgr, rule, issue := linkFixture(t)

	for i := 0; i < 2; i++ {
		if err := mgr.LinkRule(ctx, "owner", rule.Code, issue.ID, "applies_to"); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := mgr.ListIssuesOfRule(ctx, "owner", rule.Code)
	if len(got) != 1 {
		t.Fatalf("got %d links, want 1", len(got))
	}
	if err := mgr.UnlinkRule(ctx, "owner", rule.Code, issue.ID, "applies_to"); err != nil {
		t.Fatal(err)
	}
	got, _ = mgr.ListIssuesOfRule(ctx, "owner", rule.Code)
	if len(got) != 0 {
		t.Fatalf("got %d links after unlink", len(got))
	}
	if err := mgr.UnlinkRule(ctx, "owner", rule.Code, issue.ID, "applies_to"); !errors.Is(err, mwanachamataskmanager.ErrRelationshipNotFound) {
		t.Fatalf("second unlink err %v", err)
	}
}

func TestRuleLinks_RefusesWhatCannotBeLinked(t *testing.T) {
	ctx := context.Background()
	mgr, rule, issue := linkFixture(t)

	if err := mgr.LinkRule(ctx, "owner", "R-99", issue.ID, "applies_to"); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("unknown rule: %v", err)
	}
	if err := mgr.LinkRule(ctx, "owner", rule.Code, "no-such-issue", "applies_to"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("unknown issue: %v", err)
	}
	if err := mgr.LinkRule(ctx, "owner", rule.Code, issue.ID, "loves"); !errors.Is(err, mwanachamataskmanager.ErrInvalidRelationship) {
		t.Fatalf("unknown relation: %v", err)
	}
}

func TestRuleLinks_TheLinkIsAnOrdinaryRelationship(t *testing.T) {
	ctx := context.Background()
	mgr, rule, issue := linkFixture(t)
	if err := mgr.LinkRule(ctx, "owner", rule.Code, issue.ID, "applies_to"); err != nil {
		t.Fatal(err)
	}
	out, err := mgr.TraverseRelationships(ctx, rule.ID, mwanachamataskmanager.RelLabelAppliesTo, mwanachamataskmanager.DirectionOutbound)
	if err != nil || len(out) != 1 || out[0].ToID != issue.ID {
		t.Fatalf("outbound %+v, %v", out, err)
	}
	in, err := mgr.TraverseRelationships(ctx, issue.ID, mwanachamataskmanager.RelLabelAppliesTo, mwanachamataskmanager.DirectionInbound)
	if err != nil || len(in) != 1 || in[0].FromID != rule.ID {
		t.Fatalf("inbound %+v, %v", in, err)
	}
	other, _ := mgr.TraverseRelationships(ctx, rule.ID, mwanachamataskmanager.RelLabelBrokenBy, mwanachamataskmanager.DirectionOutbound)
	if len(other) != 0 {
		t.Fatalf("a different relation must not show: %+v", other)
	}
}

func TestRuleLinks_ASupersededRuleShowsItsReplacement(t *testing.T) {
	ctx := context.Background()
	mgr, rule, issue := linkFixture(t)
	activate(t, mgr, rule.Code, "in_force")
	if err := mgr.LinkRule(ctx, "owner", rule.Code, issue.ID, "applies_to"); err != nil {
		t.Fatal(err)
	}
	replacement, err := mgr.SupersedeRule(ctx, "owner", rule.Code, mwanachamataskmanager.RuleRecord{Title: "Work on main, always"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil || len(got.Linked) != 1 {
		t.Fatalf("%+v, %v", got, err)
	}
	link := got.Linked[0]
	if link.Rule.Status != "superseded" || link.Replacement == nil || link.Replacement.Code != replacement.Code {
		t.Fatalf("link %+v", link)
	}
}

func TestRuleLinks_SimilarIssuesAreOfferedTheInForceRulesTheyShareATagWith(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	tagged := func(title string, kind string, tags ...string) mwanachamataskmanager.RuleRecord {
		r, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: kind, Title: title, Tags: tags})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	inForce := tagged("In force git rule", "rule", "git")
	decided := tagged("Decided git choice", "decision", "Git")
	proposed := tagged("Proposed git rule", "rule", "git")
	retired := tagged("Retired git rule", "rule", "git")
	otherTag := tagged("A css rule", "rule", "css")
	untagged := tagged("No tag rule", "rule")
	linked := tagged("Linked git rule", "rule", "git")
	activate(t, mgr, inForce.Code, "in_force")
	activate(t, mgr, decided.Code, "decided")
	activate(t, mgr, retired.Code, "in_force")
	activate(t, mgr, retired.Code, "retired")
	activate(t, mgr, otherTag.Code, "in_force")
	activate(t, mgr, untagged.Code, "in_force")
	activate(t, mgr, linked.Code, "in_force")
	_ = proposed

	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Similar issue", Tags: []string{"git", "taskmanager"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.LinkRule(ctx, "owner", linked.Code, issue.ID, "applies_to"); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, r := range got.Suggested {
		codes = append(codes, r.Code)
	}
	if len(codes) != 2 || codes[0] != decided.Code || codes[1] != inForce.Code {
		t.Fatalf("suggested %v, want [%s %s]", codes, decided.Code, inForce.Code)
	}
}

func TestRuleLinks_TheBriefCarriesLinkedAndSuggestedRules(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	linked := mustCreateRule(t, mgr, "rule", "Linked")
	suggested, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "rule", Title: "Suggested", Tags: []string{"git"}})
	if err != nil {
		t.Fatal(err)
	}
	activate(t, mgr, suggested.Code, "in_force")
	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Brief me", Tags: []string{"git"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.LinkRule(ctx, "owner", linked.Code, issue.ID, "decided_in"); err != nil {
		t.Fatal(err)
	}
	brief, err := mgr.TaskBriefFor(ctx, mwanachamataskmanager.TaskBriefFilter{RequesterID: "owner", TaskID: issue.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Rules.Linked) != 1 || brief.Rules.Linked[0].Rule.Code != linked.Code || brief.Rules.Linked[0].Relation != "decided_in" {
		t.Fatalf("linked %+v", brief.Rules.Linked)
	}
	if len(brief.Rules.Suggested) != 1 || brief.Rules.Suggested[0].Code != suggested.Code {
		t.Fatalf("suggested %+v", brief.Rules.Suggested)
	}
}

func TestRuleLinks_RuleCodesTypedInTheDescriptionOrATodoAreRecognised(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	first := mustCreateRule(t, mgr, "rule", "First")
	second := mustCreateRule(t, mgr, "decision", "Second")
	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{
		Title: "Mentions", Description: "Follow R-1, not R-77. Also see D-1.",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, r := range got.Mentioned {
		codes = append(codes, r.Code)
	}
	if len(codes) != 2 || codes[0] != first.Code || codes[1] != second.Code {
		t.Fatalf("mentioned %v", codes)
	}
}

func TestQuestionLinks_AnOpenQuestionShowsOnTheIssueItIsLinkedToOrSharesATagWith(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	linked := mustCreateQuestion(t, mgr, "Linked", "P3")
	tagged, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "question", Title: "Tagged", Priority: "P1", Tags: []string{"git"}})
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "question", Title: "Unrelated", Priority: "P1", Tags: []string{"css"}})
	if err != nil {
		t.Fatal(err)
	}
	dropped, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "question", Title: "Dropped", Priority: "P1", Tags: []string{"git"}})
	if err != nil {
		t.Fatal(err)
	}
	activate(t, mgr, dropped.Code, "dropped")
	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Work", Tags: []string{"git"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.LinkRule(ctx, "owner", linked.Code, issue.ID, "applies_to"); err != nil {
		t.Fatal(err)
	}

	got, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if codes(got.Questions) != tagged.Code+","+linked.Code {
		t.Fatalf("questions %q, want %s,%s", codes(got.Questions), tagged.Code, linked.Code)
	}
	if len(got.Linked) != 0 || len(got.Suggested) != 0 {
		t.Fatalf("a question sits only under questions: linked %+v suggested %+v", got.Linked, got.Suggested)
	}
	_ = unrelated

	brief, err := mgr.TaskBriefFor(ctx, mwanachamataskmanager.TaskBriefFilter{RequesterID: "owner", TaskID: issue.ID})
	if err != nil {
		t.Fatal(err)
	}
	if codes(brief.Rules.Questions) != tagged.Code+","+linked.Code {
		t.Fatalf("brief questions %q", codes(brief.Rules.Questions))
	}
}

func TestQuestionLinks_AnAnsweredQuestionLeavesTheBrief(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P1")
	d := mustCreateRule(t, mgr, "decision", "This")
	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.LinkRule(ctx, "owner", q.Code, issue.ID, "applies_to"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AnswerQuestion(ctx, "owner", q.Code, d.Code); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil || len(got.Questions) != 0 {
		t.Fatalf("questions %+v, err %v", got.Questions, err)
	}
	issues, err := mgr.ListIssuesOfRule(ctx, "owner", q.Code)
	if err != nil || len(issues) != 1 {
		t.Fatalf("issues of the question: %+v, err %v", issues, err)
	}
}

func TestQuestionLinks_AQuestionCodeTypedInTheDescriptionIsRecognised(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	q := mustCreateQuestion(t, mgr, "Which?", "P1")
	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Mentions", Description: "Waits on Q-1."})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil || codes(got.Mentioned) != q.Code {
		t.Fatalf("mentioned %q, err %v", codes(got.Mentioned), err)
	}
}

func TestRuleLinks_ADecisionListsTheRulesDecidedInTheSameIssue(t *testing.T) {
	ctx := context.Background()
	mgr, decision, issue := linkFixture(t)
	decision = mustCreateRule(t, mgr, "decision", "Body is Markdown")
	produced := mustCreateRule(t, mgr, "rule", "Write Markdown")
	elsewhere := mustCreateRule(t, mgr, "rule", "Decided elsewhere")
	appliesOnly := mustCreateRule(t, mgr, "rule", "Only applies")
	otherIssue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "another"})
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []struct {
		code, issue, relation string
	}{
		{decision.Code, issue.ID, "decided_in"},
		{produced.Code, issue.ID, "decided_in"},
		{elsewhere.Code, otherIssue.ID, "decided_in"},
		{appliesOnly.Code, issue.ID, "applies_to"},
	} {
		if err := mgr.LinkRule(ctx, "owner", link.code, link.issue, link.relation); err != nil {
			t.Fatal(err)
		}
	}
	got, err := mgr.ListRulesOfDecision(ctx, "owner", decision.Code)
	if err != nil || codes(got) != produced.Code {
		t.Fatalf("got %q, err %v, want %s", codes(got), err, produced.Code)
	}
	if _, err := mgr.ListRulesOfDecision(ctx, "owner", produced.Code); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
		t.Fatalf("a rule asked for its rules: %v", err)
	}
	if _, err := mgr.ListRulesOfDecision(ctx, "owner", "D-99"); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("unknown decision: %v", err)
	}
}

func TestRuleLinks_ASupersededRuleTypedInAnIssueIsMentionedAsItsReplacement(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	old := mustCreateRule(t, mgr, "rule", "Work on a branch")
	activate(t, mgr, old.Code, "in_force")
	next, err := mgr.SupersedeRule(ctx, "owner", old.Code, mwanachamataskmanager.RuleRecord{Title: "Work on main"})
	if err != nil {
		t.Fatal(err)
	}
	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Mentions", Description: "Follow " + old.Code + " and " + next.Code + "."})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if codes(got.Mentioned) != next.Code {
		t.Fatalf("mentioned %q, want only %s", codes(got.Mentioned), next.Code)
	}
}

func TestRuleLinks_AnOpenQuestionSharingATagWithTheIssueIsNotAlsoMentioned(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	tagged, err := mgr.CreateRule(ctx, "owner", mwanachamataskmanager.RuleRecord{Kind: "question", Title: "Tagged", Priority: "P1", Tags: []string{"git"}})
	if err != nil {
		t.Fatal(err)
	}
	bare := mustCreateQuestion(t, mgr, "Bare", "P2")
	issue, err := mgr.CreateTask(ctx, mwanachamataskmanager.Task{Title: "Work", Tags: []string{"git"}, Description: "Waits on " + tagged.Code + " and " + bare.Code + "."})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ListRulesOfIssue(ctx, "owner", issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if codes(got.Questions) != tagged.Code || codes(got.Mentioned) != bare.Code {
		t.Fatalf("questions %q, mentioned %q", codes(got.Questions), codes(got.Mentioned))
	}
}
