package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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

func TestABacklogListNarrowsByTheTagQueryOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusOK, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{"tags": []string{"docs", "style"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Both", "tags": []string{"docs", "style"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Docs only", "tags": []string{"Docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Style decision", "tags": []string{"style"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "title": "Which docs?", "tags": []string{"docs"}})

	if got := f.list("ann", "/backlogs/docs/rules"); len(got) != 2 {
		t.Fatalf("rules %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/rules?tag=style"); len(got) != 1 || got[0]["code"] != "R-1" {
		t.Fatalf("rules by tag %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/rules?tag=DOCS&tag=style"); len(got) != 1 || got[0]["code"] != "R-1" {
		t.Fatalf("rules by two tags %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/rules?tag=nothing"); len(got) != 0 {
		t.Fatalf("rules by an unknown tag %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/decisions?tag=docs"); len(got) != 0 {
		t.Fatalf("decisions by a tag the decision lacks %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/decisions?tag=style"); len(got) != 1 || got[0]["code"] != "D-1" {
		t.Fatalf("decisions by tag %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/questions?tag=docs"); len(got) != 1 || got[0]["code"] != "Q-1" {
		t.Fatalf("questions by tag %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/questions?tag=style"); len(got) != 0 {
		t.Fatalf("questions by a tag the question lacks %v", got)
	}
}

func TestTheBacklogListToolsTakeATagAndNarrowByIt(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusOK, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{"tags": []string{"docs", "style"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Both", "tags": []string{"docs", "style"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Docs only", "tags": []string{"docs"}})
	for _, name := range []string{"taskmanager_backlog_rules_list", "taskmanager_backlog_decisions_list", "taskmanager_backlog_questions_list"} {
		schema, err := json.Marshal(h.tool(name).InputSchema)
		if err != nil || !strings.Contains(string(schema), `"tag"`) {
			t.Fatalf("%s schema %s, %v", name, schema, err)
		}
	}
	raw, err := json.Marshal(map[string]any{"backlogID": "docs", "tag": []string{"style"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := h.tool("taskmanager_backlog_rules_list").Invoke(context.WithValue(context.Background(), callerKey{}, "ann"), raw)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(encoded, &items); err != nil || len(items) != 1 || items[0]["code"] != "R-1" {
		t.Fatalf("got %s, %v", encoded, err)
	}
}
