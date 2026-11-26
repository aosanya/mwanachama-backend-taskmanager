package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const askedRole = "Product owner"

func askQuestion(mgr mwanachamataskmanager.TaskManager, rec mwanachamataskmanager.RuleRecord) (mwanachamataskmanager.RuleRecord, error) {
	rec.Kind = "question"
	if rec.TargetRole == "" {
		rec.TargetRole = askedRole
	}
	return mgr.CreateRule(context.Background(), "owner", rec)
}

func mustAsk(t *testing.T, mgr mwanachamataskmanager.TaskManager, rec mwanachamataskmanager.RuleRecord) mwanachamataskmanager.RuleRecord {
	t.Helper()
	q, err := askQuestion(mgr, rec)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func threeOptions() []mwanachamataskmanager.QuestionOption {
	return []mwanachamataskmanager.QuestionOption{
		{Text: "Tabs"},
		{Text: "Spaces", Recommended: true, Reason: "The formatter already writes them."},
		{Text: "Leave it to each author"},
	}
}

func TestQuestionOptions_AreKeptWithTheRecommendedOneFirst(t *testing.T) {
	mgr := newTestManager(t)
	made := mustAsk(t, mgr, mwanachamataskmanager.RuleRecord{Title: "Tabs or spaces?", Options: threeOptions()})
	read, err := mgr.ReadRule(context.Background(), "owner", made.Code)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, option := range read.Options {
		texts = append(texts, option.Text)
	}
	want := []string{"Spaces", "Tabs", "Leave it to each author"}
	if len(texts) != 3 || texts[0] != want[0] || texts[1] != want[1] || texts[2] != want[2] {
		t.Fatalf("options %v, want %v", texts, want)
	}
	if !read.Options[0].Recommended || read.Options[0].Reason != "The formatter already writes them." || read.TargetRole != askedRole {
		t.Fatalf("read %+v", read)
	}
}

func TestQuestionOptions_AQuestionNeedsATargetRole(t *testing.T) {
	mgr := newTestManager(t)
	for _, role := range []string{"", "   "} {
		_, err := mgr.CreateRule(context.Background(), "owner", mwanachamataskmanager.RuleRecord{Kind: "question", Title: "Who?", TargetRole: role})
		if !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("role %q: err %v, want ErrInvalidRule", role, err)
		}
	}
}

func TestQuestionOptions_ARuleOrDecisionTakesNeitherRoleNorOptions(t *testing.T) {
	mgr := newTestManager(t)
	for _, kind := range []string{"rule", "decision"} {
		for name, rec := range map[string]mwanachamataskmanager.RuleRecord{
			"a role":  {TargetRole: askedRole},
			"options": {Options: []mwanachamataskmanager.QuestionOption{{Text: "x"}}},
		} {
			rec.Kind, rec.Title = kind, "x"
			if _, err := mgr.CreateRule(context.Background(), "owner", rec); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
				t.Fatalf("%s with %s: err %v, want ErrInvalidRule", kind, name, err)
			}
		}
	}
}

func TestQuestionOptions_BadOptionListsAreRefused(t *testing.T) {
	mgr := newTestManager(t)
	cases := map[string][]mwanachamataskmanager.QuestionOption{
		"two recommended":                  {{Text: "a", Recommended: true, Reason: "r"}, {Text: "b", Recommended: true, Reason: "r"}},
		"recommended without a reason":     {{Text: "a", Recommended: true}},
		"a reason that is not recommended": {{Text: "a", Reason: "r"}},
		"a blank text":                     {{Text: "  "}},
		"a reason over two lines":          {{Text: "a", Recommended: true, Reason: "one\ntwo"}},
	}
	for name, options := range cases {
		if _, err := askQuestion(mgr, mwanachamataskmanager.RuleRecord{Title: name, Options: options}); !errors.Is(err, mwanachamataskmanager.ErrInvalidRule) {
			t.Fatalf("%s: err %v, want ErrInvalidRule", name, err)
		}
	}
}

func TestQuestionOptions_AQuestionFiledWithAnIssuesTagsIsListedForItsBacklogWithItsOptions(t *testing.T) {
	f := newBacklogRuleFixture(t)
	made := mustAsk(t, f.mgr, mwanachamataskmanager.RuleRecord{Title: "Tabs or spaces?", Tags: []string{"style"}, Options: threeOptions()})
	got, err := f.mgr.ListBacklogQuestions(context.Background(), "owner", f.backlog, mwanachamataskmanager.RuleFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Code != made.Code || len(got[0].Options) != 3 || got[0].TargetRole != askedRole {
		t.Fatalf("got %+v", got)
	}
}

func roleOfKind(kind string) string {
	if kind == "question" {
		return askedRole
	}
	return ""
}
