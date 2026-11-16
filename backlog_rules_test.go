package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

var rulesNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func rule(field, op, value string) mwanachamataskmanager.Rule {
	return mwanachamataskmanager.Rule{Field: field, Operator: op, Value: value}
}

func ruleSet(match string, rules ...mwanachamataskmanager.Rule) mwanachamataskmanager.RuleSet {
	return mwanachamataskmanager.RuleSet{Match: match, Rules: rules}
}

func TestRuleSet_ValidateRefusals(t *testing.T) {
	cases := []struct {
		name string
		set  mwanachamataskmanager.RuleSet
		want string
	}{
		{"no rules", ruleSet("all"), "at least one rule"},
		{"unknown field", ruleSet("all", rule("colour", "is", "red")), "unknown field"},
		{"wrong operator for the field", ruleSet("all", rule("tag", "is", "Docs")), "does not take"},
		{"title with a tag operator", ruleSet("all", rule("title", "has", "x")), "does not take"},
		{"empty value", ruleSet("all", rule("tag", "has", "  ")), "needs a value"},
		{"unknown mark", ruleSet("all", rule("mark", "is", "Stale")), "Ready or Blocked"},
		{"unknown priority", ruleSet("all", rule("priority", "is", "urgent")), "low, medium, high or critical"},
		{"waits on with a value", ruleSet("all", rule("waits_on", "has_something", "x")), "takes no value"},
		{"span without a unit", ruleSet("all", rule("last_changed", "in_the_last", "30")), "number and a unit"},
		{"span with a bad unit", ruleSet("all", rule("last_changed", "in_the_last", "3 years")), "days, weeks or months"},
		{"span of zero", ruleSet("all", rule("last_changed", "in_the_last", "0 days")), "above zero"},
		{"unknown match", ruleSet("some", rule("tag", "has", "Docs")), "all or any"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.set.Validate()
			if !errors.Is(err, mwanachamataskmanager.ErrInvalidRules) {
				t.Fatalf("got %v, want ErrInvalidRules", err)
			}
			if !contains(err.Error(), c.want) {
				t.Fatalf("got %q, want it to mention %q", err.Error(), c.want)
			}
		})
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRuleSet_AcceptsEveryFieldAndOperator(t *testing.T) {
	set := ruleSet("any",
		rule("tag", "has", "Docs"), rule("tag", "does_not_have", "Docs"),
		rule("mark", "is", "Ready"), rule("mark", "is_not", "Blocked"),
		rule("backlog", "is", "Software"), rule("backlog", "is_not", "Software"),
		rule("step", "is", "To do"), rule("step", "is_not", "To do"),
		rule("priority", "is", "high"), rule("priority", "is_not", "low"),
		rule("title", "contains", "fix"), rule("title", "does_not_contain", "fix"),
		rule("description", "contains", "fix"), rule("description", "does_not_contain", "fix"),
		rule("waits_on", "has_something", ""), rule("waits_on", "has_nothing", ""),
		rule("last_changed", "in_the_last", "30 days"), rule("last_changed", "not_in_the_last", "2 weeks"),
	)
	if err := set.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestRuleSet_NormaliseGivesEqualKeysForEqualRules(t *testing.T) {
	a := ruleSet("ALL", rule("Tag", "Has", " Ready "), rule("Step", "is", "To do"), rule("last changed", "in the last", "1 Months"))
	b := ruleSet("", rule("last_changed", "in_the_last", "1 month"), rule("step", "is", "to DO"), rule("tag", "has", "ready"), rule("tag", "has", "READY"))
	ka, err := a.Key()
	if err != nil {
		t.Fatalf("Key a: %v", err)
	}
	kb, err := b.Key()
	if err != nil {
		t.Fatalf("Key b: %v", err)
	}
	if ka != kb {
		t.Fatalf("keys differ:\n%s\n%s", ka, kb)
	}
}

func TestRuleSet_NormaliseKeepsDifferentRulesApart(t *testing.T) {
	a := ruleSet("all", rule("tag", "has", "Docs"))
	b := ruleSet("any", rule("tag", "has", "Docs"))
	c := ruleSet("all", rule("tag", "does_not_have", "Docs"))
	keys := map[string]bool{}
	for _, s := range []mwanachamataskmanager.RuleSet{a, b, c} {
		k, err := s.Key()
		if err != nil {
			t.Fatalf("Key: %v", err)
		}
		keys[k] = true
	}
	if len(keys) != 3 {
		t.Fatalf("want three different keys, got %v", keys)
	}
}

func TestRuleSet_NormaliseOrdersByFieldThenOperatorThenValue(t *testing.T) {
	set, err := ruleSet("all", rule("last_changed", "in_the_last", "5 days"), rule("tag", "has", "b"), rule("tag", "has", "a"), rule("mark", "is", "ready")).Normalise()
	if err != nil {
		t.Fatalf("Normalise: %v", err)
	}
	var got []string
	for _, r := range set.Rules {
		got = append(got, r.Field+":"+r.Value)
	}
	want := []string{"tag:a", "tag:b", "mark:Ready", "last_changed:5 days"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func subject(task mwanachamataskmanager.Task) mwanachamataskmanager.RuleSubject {
	return mwanachamataskmanager.RuleSubject{Task: task, BacklogName: "Software", StepName: "To do"}
}

func TestRuleSet_Matches(t *testing.T) {
	recent := rulesNow.AddDate(0, 0, -3).Format(time.RFC3339)
	old := rulesNow.AddDate(0, 0, -45).Format(time.RFC3339)
	base := mwanachamataskmanager.Task{
		Title: "Fix mirrored board drift", Description: "Compare the boards", Priority: mwanachamataskmanager.TaskPriorityHigh,
		Tags: []string{"Docs", "Ready"}, ProjectName: "hub-steward", Backlog: "b-1", Step: "s-1", UpdatedAt: recent,
	}
	blocked := base
	blocked.Tags = []string{"Docs", "Blocked"}
	stale := base
	stale.UpdatedAt = old
	waiting := subject(base)
	waiting.WaitsOn = 2

	cases := []struct {
		name string
		set  mwanachamataskmanager.RuleSet
		in   mwanachamataskmanager.RuleSubject
		want bool
	}{
		{"tag has, any case", ruleSet("all", rule("tag", "has", "docs")), subject(base), true},
		{"tag has the repo", ruleSet("all", rule("tag", "has", "hub-steward")), subject(base), true},
		{"tag has missing", ruleSet("all", rule("tag", "has", "Other")), subject(base), false},
		{"tag does not have", ruleSet("all", rule("tag", "does_not_have", "Other")), subject(base), true},
		{"mark ready", ruleSet("all", rule("mark", "is", "Ready")), subject(base), true},
		{"mark ready is false when blocked", ruleSet("all", rule("mark", "is", "Ready")), subject(blocked), false},
		{"mark blocked", ruleSet("all", rule("mark", "is", "Blocked")), subject(blocked), true},
		{"mark is not blocked", ruleSet("all", rule("mark", "is_not", "Blocked")), subject(base), true},
		{"a blocked status without the tag is not Blocked", ruleSet("all", rule("mark", "is", "Blocked")), subject(mwanachamataskmanager.Task{Status: mwanachamataskmanager.TaskStatusBlocked}), false},
		{"a blocked status without the tag is not Ready", ruleSet("all", rule("mark", "is", "Ready")), subject(mwanachamataskmanager.Task{Status: mwanachamataskmanager.TaskStatusBlocked}), false},
		{"a paused row with neither tag is not Ready", ruleSet("all", rule("mark", "is", "Ready")), subject(mwanachamataskmanager.Task{Tags: []string{"Docs"}}), false},
		{"a paused row with neither tag is not Blocked", ruleSet("all", rule("mark", "is", "Blocked")), subject(mwanachamataskmanager.Task{Tags: []string{"Docs"}}), false},
		{"a paused row with neither tag is not Ready, as a negative rule", ruleSet("all", rule("mark", "is_not", "Ready")), subject(mwanachamataskmanager.Task{Tags: []string{"Docs"}}), true},
		{"Blocked wins when both tags are present", ruleSet("all", rule("mark", "is", "Ready")), subject(mwanachamataskmanager.Task{Tags: []string{"Ready", "Blocked"}}), false},
		{"backlog by name", ruleSet("all", rule("backlog", "is", "software")), subject(base), true},
		{"backlog by id", ruleSet("all", rule("backlog", "is", "b-1")), subject(base), true},
		{"backlog is not", ruleSet("all", rule("backlog", "is_not", "Documentation")), subject(base), true},
		{"step by name", ruleSet("all", rule("step", "is", "To do")), subject(base), true},
		{"step is not", ruleSet("all", rule("step", "is_not", "To do")), subject(base), false},
		{"priority", ruleSet("all", rule("priority", "is", "HIGH")), subject(base), true},
		{"priority is not", ruleSet("all", rule("priority", "is_not", "high")), subject(base), false},
		{"title contains", ruleSet("all", rule("title", "contains", "MIRRORED")), subject(base), true},
		{"title does not contain", ruleSet("all", rule("title", "does_not_contain", "mirrored")), subject(base), false},
		{"description contains", ruleSet("all", rule("description", "contains", "boards")), subject(base), true},
		{"description does not match a title word", ruleSet("all", rule("description", "contains", "drift")), subject(base), false},
		{"waits on nothing", ruleSet("all", rule("waits_on", "has_nothing", "")), subject(base), true},
		{"waits on something", ruleSet("all", rule("waits_on", "has_something", "")), waiting, true},
		{"changed in the last 30 days", ruleSet("all", rule("last_changed", "in_the_last", "30 days")), subject(base), true},
		{"not changed in the last 30 days", ruleSet("all", rule("last_changed", "not_in_the_last", "30 days")), subject(stale), true},
		{"stale is not in the last 30 days only", ruleSet("all", rule("last_changed", "in_the_last", "30 days")), subject(stale), false},
		{"changed in the last 2 months", ruleSet("all", rule("last_changed", "in_the_last", "2 months")), subject(stale), true},
		{"unparsable update time is never recent", ruleSet("all", rule("last_changed", "in_the_last", "30 days")), subject(mwanachamataskmanager.Task{}), false},
		{"all needs every rule", ruleSet("all", rule("tag", "has", "Docs"), rule("step", "is", "Doing")), subject(base), false},
		{"any needs one rule", ruleSet("any", rule("tag", "has", "Other"), rule("step", "is", "To do")), subject(base), true},
		{"any with none matching", ruleSet("any", rule("tag", "has", "Other"), rule("step", "is", "Doing")), subject(base), false},
		{"ready to start", ruleSet("all", rule("mark", "is", "Ready"), rule("step", "is", "To do")), subject(base), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.set.Matches(c.in, rulesNow)
			if err != nil {
				t.Fatalf("Matches: %v", err)
			}
			if got != c.want {
				t.Fatalf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestRuleSet_MatchesRefusesInvalidRules(t *testing.T) {
	_, err := ruleSet("all", rule("tag", "is", "x")).Matches(subject(mwanachamataskmanager.Task{}), rulesNow)
	if !errors.Is(err, mwanachamataskmanager.ErrInvalidRules) {
		t.Fatalf("got %v, want ErrInvalidRules", err)
	}
}

func TestRuleSet_TagRuleAgreesWithTheListFilter(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "docs one", Tags: []string{"Docs"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "docs two", Tags: []string{"docs", "P1"}})
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "plain"})

	tasks, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	set := ruleSet("all", rule("tag", "has", "Docs"))
	got := map[string]bool{}
	for _, task := range tasks {
		ok, err := set.Matches(subject(task), time.Now())
		if err != nil {
			t.Fatalf("Matches: %v", err)
		}
		if ok {
			got[task.Title] = true
		}
	}
	wantTitles(t, got, "docs one", "docs two")
	wantTitles(t, listTitles(t, mgr, mwanachamataskmanager.TaskFilter{Tag: []string{"Docs"}}), "docs one", "docs two")
}

func TestRuleSet_LastChangedSpansUseTheirUnit(t *testing.T) {
	tenDays := mwanachamataskmanager.Task{UpdatedAt: rulesNow.AddDate(0, 0, -10).Format(time.RFC3339)}
	fortyDays := mwanachamataskmanager.Task{UpdatedAt: rulesNow.AddDate(0, 0, -40).Format(time.RFC3339)}
	cases := []struct {
		name   string
		task   mwanachamataskmanager.Task
		span   string
		inLast bool
	}{
		{"ten days is not in the last week", tenDays, "1 week", false},
		{"ten days is in the last two weeks", tenDays, "2 weeks", true},
		{"forty days is not in the last month", fortyDays, "1 month", false},
		{"forty days is in the last two months", fortyDays, "2 months", true},
		{"forty days is not in the last thirty days", fortyDays, "30 days", false},
	}
	for _, c := range cases {
		got, err := ruleSet("all", rule("last_changed", "in_the_last", c.span)).Matches(subject(c.task), rulesNow)
		if err != nil || got != c.inLast {
			t.Errorf("%s: got %v %v", c.name, got, err)
		}
	}
}
