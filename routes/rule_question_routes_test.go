package routes_test

import (
	"net/http"
	"testing"
)

func TestAQuestionIsAskedPrioritisedAnsweredAndListedOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h

	plain := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "Is body Markdown?", "tags": []string{"docs"}})
	if plain["code"] != "Q-1" || plain["status"] != "open" || plain["priority"] != "P2" {
		t.Fatalf("got %v", plain)
	}
	urgent := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "Who signs off?", "priority": "P1"})
	if urgent["code"] != "Q-2" || urgent["priority"] != "P1" {
		t.Fatalf("got %v", urgent)
	}
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Body is Markdown"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Write plainly"})

	badPriorities := []struct {
		name string
		kind string
		prio string
	}{
		{"a question with an unknown priority", "question", "P9"},
		{"a rule with a priority", "rule", "P1"},
		{"a decision with a priority", "decision", "P2"},
	}
	for _, c := range badPriorities {
		t.Run(c.name, func(t *testing.T) {
			sub := *h
			sub.t = t
			sub.expect(http.StatusBadRequest, "bob", "POST", "/rules", map[string]any{"kind": c.kind, "title": "x", "priority": c.prio})
		})
	}

	moved := h.expect(http.StatusOK, "bob", "PATCH", "/rules/Q-1", map[string]any{"priority": "P3"})
	if moved["priority"] != "P3" {
		t.Fatalf("got %v", moved)
	}
	h.expect(http.StatusBadRequest, "bob", "PATCH", "/rules/Q-1", map[string]any{"priority": "high"})
	h.expect(http.StatusBadRequest, "bob", "PATCH", "/rules/R-1", map[string]any{"priority": "P1"})

	if got := f.list("ann", "/rules?kind=question"); len(got) != 2 || got[0]["code"] != "Q-2" || got[1]["code"] != "Q-1" {
		t.Fatalf("open questions P1 first: %v", got)
	}

	answerCases := []struct {
		name   string
		caller string
		code   string
		body   map[string]any
		want   int
	}{
		{"an unknown decision code", "bob", "Q-1", map[string]any{"decision_code": "D-99"}, http.StatusNotFound},
		{"an unknown question code", "bob", "Q-99", map[string]any{"decision_code": "D-1"}, http.StatusNotFound},
		{"a rule named as the answer", "bob", "Q-1", map[string]any{"decision_code": "R-1"}, http.StatusBadRequest},
		{"a decision answered instead of a question", "bob", "D-1", map[string]any{"decision_code": "D-1"}, http.StatusBadRequest},
		{"a missing decision code", "bob", "Q-1", map[string]any{}, http.StatusBadRequest},
		{"a reader", "ann", "Q-1", map[string]any{"decision_code": "D-1"}, http.StatusForbidden},
		{"a real decision", "bob", "Q-1", map[string]any{"decision_code": "D-1"}, http.StatusOK},
	}
	for _, c := range answerCases {
		t.Run(c.name, func(t *testing.T) {
			sub := *h
			sub.t = t
			got := sub.expect(c.want, c.caller, "PUT", "/rules/"+c.code+"/answer", c.body)
			if c.want == http.StatusOK && (got["status"] != "answered" || got["answered_by"] != "D-1") {
				t.Fatalf("got %v", got)
			}
		})
	}

	if read := h.expect(http.StatusOK, "ann", "GET", "/rules/Q-1", nil); read["answered_by"] != "D-1" {
		t.Fatalf("got %v", read)
	}
	if got := f.list("ann", "/rules?kind=question"); len(got) != 1 || got[0]["code"] != "Q-2" {
		t.Fatalf("answered question left the default list: %v", got)
	}
	if got := f.list("ann", "/rules?kind=question&status=answered"); len(got) != 1 || got[0]["code"] != "Q-1" {
		t.Fatalf("by status %v", got)
	}
	h.expect(http.StatusOK, "bob", "PUT", "/rules/Q-2/status", map[string]any{"status": "dropped"})
	if got := f.list("ann", "/rules?kind=question&status=dropped"); len(got) != 1 || got[0]["code"] != "Q-2" {
		t.Fatalf("dropped %v", got)
	}
	h.expect(http.StatusBadRequest, "ann", "GET", "/rules?kind=question&status=in_force", nil)
	h.expect(http.StatusBadRequest, "bob", "PUT", "/rules/Q-2/status", map[string]any{"status": "answered"})
}

func TestTheRulesOfAnIssueCarryItsOpenQuestionsOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "Linked one", "priority": "P3"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "Unrelated one"})
	h.expect(http.StatusNoContent, "bob", "POST", "/rules/Q-1/links", map[string]any{"issue_id": f.issue, "relation": "applies_to"})

	read := h.expect(http.StatusOK, "ann", "GET", "/tasks/"+f.issue+"/rules", nil)
	questions, _ := read["questions"].([]any)
	if len(questions) != 1 || questions[0].(map[string]any)["code"] != "Q-1" {
		t.Fatalf("got %v", read)
	}
	if linked, _ := read["linked"].([]any); len(linked) != 0 {
		t.Fatalf("a question is never under linked: %v", read)
	}

	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Settled"})
	h.expect(http.StatusOK, "bob", "PUT", "/rules/Q-1/answer", map[string]any{"decision_code": "D-1"})
	after := h.expect(http.StatusOK, "ann", "GET", "/tasks/"+f.issue+"/rules", nil)
	if left, _ := after["questions"].([]any); len(left) != 0 {
		t.Fatalf("an answered question leaves the brief: %v", after)
	}
}

func TestABacklogRuleListNeverHoldsQuestionsOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusOK, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{"tags": []string{"docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "Open", "tags": []string{"docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Plain", "tags": []string{"docs"}})

	if got := f.list("ann", "/backlogs/docs/rules"); len(got) != 1 || got[0]["code"] != "R-1" {
		t.Fatalf("rules %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/decisions"); len(got) != 0 {
		t.Fatalf("decisions %v", got)
	}
	h.expect(http.StatusBadRequest, "ann", "GET", "/backlogs/docs/rules?status=open", nil)
	h.expect(http.StatusBadRequest, "ann", "GET", "/backlogs/docs/decisions?status=open", nil)
}
