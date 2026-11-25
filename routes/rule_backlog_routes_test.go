package routes_test

import (
	"net/http"
	"testing"
)

func TestABacklogListsItsRulesAndDecisionsByTagOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Secret", "steps": []string{"Open"}})
	h.expect(http.StatusOK, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{"tags": []string{"docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Write plainly", "tags": []string{"Docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Unrelated", "tags": []string{"infra"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Use plain words", "tags": []string{"docs"}})

	if got := f.list("ann", "/backlogs/docs/rules"); len(got) != 1 || got[0]["code"] != "R-1" {
		t.Fatalf("rules %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/decisions"); len(got) != 1 || got[0]["code"] != "D-1" {
		t.Fatalf("decisions %v", got)
	}
	h.expect(http.StatusOK, "bob", "PUT", "/rules/R-1/status", map[string]any{"status": "retired"})
	if got := f.list("ann", "/backlogs/docs/rules"); len(got) != 0 {
		t.Fatalf("retired rule shown by default: %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/rules?status=retired"); len(got) != 1 {
		t.Fatalf("retired rule hidden when asked for: %v", got)
	}
	h.expect(http.StatusBadRequest, "ann", "GET", "/backlogs/docs/rules?status=decided", nil)
	h.expect(http.StatusBadRequest, "ann", "GET", "/backlogs/docs/decisions?status=in_force", nil)
	h.expect(http.StatusBadRequest, "ann", "GET", "/backlogs/nope/rules", nil)
	h.expect(http.StatusForbidden, "ann", "GET", "/backlogs/secret/rules", nil)
	h.expect(http.StatusForbidden, "ann", "GET", "/backlogs/secret/decisions", nil)
}
