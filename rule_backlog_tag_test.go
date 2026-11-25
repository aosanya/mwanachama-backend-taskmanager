package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestBacklogRuleLists_ATagNarrowsEachListToRecordsCarryingEveryTagGiven(t *testing.T) {
	ctx := context.Background()
	f := newBacklogRuleFixture(t)
	lists := map[string]func(mwanachamataskmanager.RuleFilter) ([]mwanachamataskmanager.RuleRecord, error){
		"rule": func(filter mwanachamataskmanager.RuleFilter) ([]mwanachamataskmanager.RuleRecord, error) {
			return f.mgr.ListBacklogRules(ctx, "owner", f.backlog, filter)
		},
		"decision": func(filter mwanachamataskmanager.RuleFilter) ([]mwanachamataskmanager.RuleRecord, error) {
			return f.mgr.ListBacklogDecisions(ctx, "owner", f.backlog, filter)
		},
		"question": func(filter mwanachamataskmanager.RuleFilter) ([]mwanachamataskmanager.RuleRecord, error) {
			return f.mgr.ListBacklogQuestions(ctx, "owner", f.backlog, filter)
		},
	}
	cases := []struct {
		name string
		tags []string
		want []int
	}{
		{"no tag keeps the whole scope", nil, []int{0, 1, 2}},
		{"one tag", []string{"data"}, []int{0, 2}},
		{"without regard to case", []string{"STYLE"}, []int{0, 1}},
		{"every tag given", []string{"style", "data"}, []int{0}},
		{"a tag nothing in scope carries", []string{"nothing"}, nil},
		{"a tag only an out-of-scope record carries", []string{"infra"}, nil},
	}
	for kind, list := range lists {
		made := []mwanachamataskmanager.RuleRecord{
			f.record(t, kind, "both "+kind, "style", "data"),
			f.record(t, kind, "style only "+kind, "style"),
			f.record(t, kind, "data only "+kind, "data"),
		}
		f.record(t, kind, "infra "+kind, "infra")
		for _, c := range cases {
			got, err := list(mwanachamataskmanager.RuleFilter{Tags: c.tags})
			if err != nil {
				t.Fatalf("%s %s: %v", kind, c.name, err)
			}
			var want []mwanachamataskmanager.RuleRecord
			for _, i := range c.want {
				want = append(want, made[i])
			}
			if codes(got) != codes(want) {
				t.Fatalf("%s: %s: got %q, want %q", kind, c.name, codes(got), codes(want))
			}
		}
	}
}
