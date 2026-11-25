package routes_test

import (
	"net/http"
	"testing"
)

func newRuleOverlapFixture(t *testing.T) *ruleFixture {
	t.Helper()
	f := newRuleFixture(t)
	f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "style it", "tags": []string{"css", "app"}, "backlog": "docs", "step": "write"})
	for _, tag := range []string{"css", "app"} {
		made := f.h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{
			"kind": "rule", "title": "rule " + tag,
			"applies_when": map[string]any{"match": "all", "rules": []map[string]any{{"field": "tag", "operator": "has", "value": tag}}},
		})
		f.h.expect(http.StatusOK, "bob", "PUT", "/rules/"+made["code"].(string)+"/status", map[string]any{"status": "in_force"})
	}
	return f
}

func TestTwoRulesThatGovernTheSameIssueAreListedAsAnOverlapOverTheRoutes(t *testing.T) {
	f := newRuleOverlapFixture(t)

	got := f.list("ann", "/rules/overlaps")

	if len(got) != 1 {
		t.Fatalf("overlaps %v", got)
	}
	first, second := got[0]["first"].(map[string]any), got[0]["second"].(map[string]any)
	if first["code"] != "R-1" || second["code"] != "R-2" {
		t.Fatalf("pair %v", got[0])
	}
	reasons := got[0]["reasons"].([]any)
	if len(reasons) != 1 || reasons[0] != "shared_issue" {
		t.Fatalf("reasons %v", reasons)
	}
}

func TestTheOverlapListNeedsCanSeeAndIsNotMistakenForARuleCode(t *testing.T) {
	f := newRuleOverlapFixture(t)

	f.h.expect(http.StatusForbidden, "nog", "GET", "/rules/overlaps", nil)
	f.h.expect(http.StatusOK, "ann", "GET", "/rules/R-1", nil)
	f.h.expect(http.StatusNotFound, "ann", "GET", "/rules/R-77", nil)
}

func TestAnAppliesWhenSetRidesOnTheRuleRoutesAndInvalidOnesAre400(t *testing.T) {
	f := newRuleFixture(t)
	f.h.expect(http.StatusBadRequest, "bob", "POST", "/rules", map[string]any{
		"kind": "rule", "title": "x",
		"applies_when": map[string]any{"match": "all", "rules": []map[string]any{{"field": "colour", "operator": "is", "value": "red"}}},
	})
	made := f.h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{
		"kind": "rule", "title": "x",
		"applies_when": map[string]any{"match": "ALL", "rules": []map[string]any{{"field": "Tag", "operator": "Has", "value": "CSS"}}},
	})
	if when, _ := made["applies_when"].(map[string]any); when["match"] != "all" {
		t.Fatalf("created %v", made)
	}
	f.h.expect(http.StatusBadRequest, "bob", "PATCH", "/rules/R-1", map[string]any{"applies_when": map[string]any{"match": "all", "rules": []any{}}})
	cleared := f.h.expect(http.StatusOK, "bob", "PATCH", "/rules/R-1", map[string]any{"applies_when": map[string]any{}})
	if _, has := cleared["applies_when"]; has {
		t.Fatalf("cleared %v", cleared)
	}
}

func TestTheOverlapToolIsNamedAfterItsAction(t *testing.T) {
	h := newAccessHub(t, map[string][]string{})
	if h.tool("taskmanager_rule_overlap_list").Description == "" {
		t.Fatal("tool taskmanager_rule_overlap_list has no description")
	}
}
