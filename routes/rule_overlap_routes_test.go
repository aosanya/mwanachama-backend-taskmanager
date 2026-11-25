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

	got := f.h.expect(http.StatusOK, "ann", "GET", "/rules/overlaps", nil)["items"].([]any)

	if len(got) != 1 {
		t.Fatalf("overlaps %v", got)
	}
	pair := got[0].(map[string]any)
	first, second := pair["first"].(map[string]any), pair["second"].(map[string]any)
	if first["code"] != "R-1" || second["code"] != "R-2" {
		t.Fatalf("pair %v", pair)
	}
	reasons := pair["reasons"].([]any)
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

func TestACompatiblePairIsAddedReadHiddenAndRemovedOverTheRoutes(t *testing.T) {
	f := newRuleOverlapFixture(t)

	f.h.expect(http.StatusNoContent, "bob", "POST", "/rules/R-1/compatible/R-2", nil)
	f.h.expect(http.StatusConflict, "bob", "POST", "/rules/R-2/compatible/R-1", nil)
	f.h.expect(http.StatusBadRequest, "bob", "POST", "/rules/R-1/compatible/R-1", nil)
	f.h.expect(http.StatusForbidden, "nog", "POST", "/rules/R-1/compatible/R-2", nil)

	list := f.h.expect(http.StatusOK, "ann", "GET", "/rules/overlaps", nil)
	if len(list["items"].([]any)) != 0 || list["hidden_compatible"] != float64(1) {
		t.Fatalf("overlaps %v", list)
	}
	read := f.h.expect(http.StatusOK, "ann", "GET", "/rules/R-2", nil)
	if got := read["compatible"].([]any); len(got) != 1 || got[0] != "R-1" {
		t.Fatalf("read %v", read)
	}

	f.h.expect(http.StatusNoContent, "bob", "DELETE", "/rules/R-2/compatible/R-1", nil)
	f.h.expect(http.StatusNotFound, "bob", "DELETE", "/rules/R-2/compatible/R-1", nil)
	if list := f.h.expect(http.StatusOK, "ann", "GET", "/rules/overlaps", nil); len(list["items"].([]any)) != 1 {
		t.Fatalf("after remove %v", list)
	}
}
