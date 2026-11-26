package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type pathsGone map[string]bool

func (g pathsGone) PathExists(_ context.Context, _, path string) (bool, error) {
	return !g[path], nil
}

type ruleFixture struct {
	h     *accessHub
	issue string
}

func newRuleFixture(t *testing.T) *ruleFixture {
	t.Helper()
	return newRuleFixtureWithChecker(t, nil)
}

func newRuleFixtureWithChecker(t *testing.T, checker mwanachamataskmanager.RepoPathChecker) *ruleFixture {
	t.Helper()
	h := newAccessHubWithChecker(t, ":memory:", map[string][]string{"ann": {"readers"}, "bob": {"writers"}, "boss": {"owners"}, "nog": {"nobody"}}, checker)
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	docs := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write", "Done"}})["id"].(string)
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/readers", map[string]string{"level": "see"})
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/writers", map[string]string{"level": "move"})
	issue := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "write the guide", "backlog": docs, "step": "write"})["id"].(string)
	return &ruleFixture{h: h, issue: issue}
}

func (f *ruleFixture) list(caller, path string) []map[string]any {
	f.h.t.Helper()
	code, raw := f.h.rawGet(caller, path)
	if code != http.StatusOK {
		f.h.t.Fatalf("GET %s as %q = %d %s", path, caller, code, raw)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		f.h.t.Fatalf("GET %s: %v: %s", path, err, raw)
	}
	return out
}

func TestARuleIsCreatedReadUpdatedAndListedByTagOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h

	made := h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "No comments in code", "tags": []string{"Style"}})
	if made["code"] != "R-1" || made["status"] != "proposed" {
		t.Fatalf("got %v", made)
	}
	h.expect(http.StatusBadRequest, "bob", "POST", "/rules", map[string]any{"kind": "law", "title": "x"})

	read := h.expect(http.StatusOK, "ann", "GET", "/rules/R-1", nil)
	if read["title"] != "No comments in code" {
		t.Fatalf("got %v", read)
	}
	h.expect(http.StatusNotFound, "ann", "GET", "/rules/R-99", nil)

	updated := h.expect(http.StatusOK, "bob", "PATCH", "/rules/R-1", map[string]any{"body": "A name carries it."})
	if updated["body"] != "A name carries it." || updated["title"] != "No comments in code" {
		t.Fatalf("got %v", updated)
	}

	h.expect(http.StatusOK, "bob", "PUT", "/rules/R-1/status", map[string]any{"status": "in_force"})
	h.expect(http.StatusBadRequest, "bob", "PUT", "/rules/R-1/status", map[string]any{"status": "decided"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Use GORM", "tags": []string{"data"}})

	if got := f.list("ann", "/rules?tag=style"); len(got) != 1 || got[0]["code"] != "R-1" {
		t.Fatalf("by tag %v", got)
	}
	if got := f.list("ann", "/rules?kind=decision&status=proposed"); len(got) != 1 || got[0]["code"] != "D-1" {
		t.Fatalf("by kind %v", got)
	}
	if got := f.list("ann", "/rules"); len(got) != 2 {
		t.Fatalf("default list holds in-force, decided and proposed: %v", got)
	}
	h.expect(http.StatusBadRequest, "ann", "GET", "/rules?kind=law", nil)
}

func TestAReplacementSupersedesTheOldRuleOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Old"})
	h.expect(http.StatusOK, "bob", "PUT", "/rules/R-1/status", map[string]any{"status": "in_force"})

	next := h.expect(http.StatusCreated, "bob", "POST", "/rules/R-1/supersede", map[string]any{"kind": "rule", "title": "New"})
	if next["code"] != "R-2" || next["supersedes"] != "R-1" {
		t.Fatalf("got %v", next)
	}
	if old := h.expect(http.StatusOK, "ann", "GET", "/rules/R-1", nil); old["status"] != "superseded" || old["superseded_by"] != "R-2" {
		t.Fatalf("got %v", old)
	}
	h.expect(http.StatusBadRequest, "bob", "POST", "/rules/R-1/supersede", map[string]any{"kind": "rule", "title": "Again"})
}

func TestARuleIsLinkedToAnIssueAndReadFromBothSidesOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Write plainly"})

	h.expect(http.StatusNoContent, "bob", "POST", "/rules/R-1/links", map[string]any{"issue_id": f.issue, "relation": "applies_to"})
	h.expect(http.StatusBadRequest, "bob", "POST", "/rules/R-1/links", map[string]any{"issue_id": f.issue, "relation": "likes"})
	h.expect(http.StatusNotFound, "bob", "POST", "/rules/R-9/links", map[string]any{"issue_id": f.issue, "relation": "applies_to"})

	issues := f.list("ann", "/rules/R-1/issues")
	if len(issues) != 1 || issues[0]["issue_id"] != f.issue || issues[0]["relation"] != "applies_to" {
		t.Fatalf("got %v", issues)
	}
	read := h.expect(http.StatusOK, "ann", "GET", "/tasks/"+f.issue+"/rules", nil)
	linked, _ := read["linked"].([]any)
	if len(linked) != 1 {
		t.Fatalf("got %v", read)
	}
	h.expect(http.StatusNotFound, "nog", "GET", "/tasks/"+f.issue+"/rules", nil)

	h.expect(http.StatusNoContent, "bob", "DELETE", "/rules/R-1/links/"+f.issue+"/applies_to", nil)
	if left := f.list("ann", "/rules/R-1/issues"); len(left) != 0 {
		t.Fatalf("got %v", left)
	}
}

func TestAFileLinkIsAddedListedAndRemovedOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Thin routes"})

	added := h.expect(http.StatusCreated, "bob", "POST", "/rules/R-1/files", map[string]any{"repo": "hub", "path": "routes/rules.go", "line": 12, "note": "the model"})
	file, _ := added["file"].(map[string]any)
	if file["path"] != "routes/rules.go" || file["id"] == "" {
		t.Fatalf("got %v", added)
	}
	h.expect(http.StatusConflict, "bob", "POST", "/rules/R-1/files", map[string]any{"repo": "hub", "path": "routes/rules.go", "line": 12})
	h.expect(http.StatusBadRequest, "bob", "POST", "/rules/R-1/files", map[string]any{"path": "a.go"})

	if got := f.list("ann", "/rules/R-1/files"); len(got) != 1 {
		t.Fatalf("got %v", got)
	}
	id := file["id"].(string)
	h.expect(http.StatusNoContent, "bob", "DELETE", "/rules/R-1/files/"+id, nil)
	h.expect(http.StatusNotFound, "bob", "DELETE", "/rules/R-1/files/"+id, nil)
}

func TestAReaderCannotWriteRulesAndAStrangerCannotReadThem(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Write plainly"})

	h.expect(http.StatusForbidden, "ann", "POST", "/rules", map[string]any{"kind": "rule", "title": "Sneaky"})
	h.expect(http.StatusForbidden, "ann", "PATCH", "/rules/R-1", map[string]any{"body": "x"})
	h.expect(http.StatusForbidden, "ann", "PUT", "/rules/R-1/status", map[string]any{"status": "in_force"})
	h.expect(http.StatusForbidden, "ann", "POST", "/rules/R-1/supersede", map[string]any{"kind": "rule", "title": "x"})
	h.expect(http.StatusForbidden, "ann", "POST", "/rules/R-1/links", map[string]any{"issue_id": f.issue, "relation": "applies_to"})
	h.expect(http.StatusForbidden, "ann", "DELETE", "/rules/R-1/links/"+f.issue+"/applies_to", nil)
	h.expect(http.StatusForbidden, "ann", "POST", "/rules/R-1/files", map[string]any{"repo": "hub", "path": "a.go"})
	h.expect(http.StatusForbidden, "ann", "DELETE", "/rules/R-1/files/x", nil)

	h.expect(http.StatusOK, "ann", "GET", "/rules/R-1", nil)
	h.expect(http.StatusForbidden, "nog", "GET", "/rules/R-1", nil)
	h.expect(http.StatusForbidden, "nog", "GET", "/rules", nil)
	h.expect(http.StatusForbidden, "nog", "GET", "/rules/R-1/files", nil)
}

func TestTheRuleToolsAreNamedAfterTheirActions(t *testing.T) {
	h := newAccessHub(t, map[string][]string{})
	for _, name := range []string{
		"taskmanager_rule_create", "taskmanager_rule_read", "taskmanager_rule_update", "taskmanager_rule_list",
		"taskmanager_rule_set_status", "taskmanager_rule_answer", "taskmanager_rule_supersede", "taskmanager_rule_link", "taskmanager_rule_unlink",
		"taskmanager_rule_issue_list", "taskmanager_rule_list_for_issue",
		"taskmanager_rule_file_add", "taskmanager_rule_file_list", "taskmanager_rule_file_remove",
		"taskmanager_rule_compatible_add", "taskmanager_rule_compatible_remove",
	} {
		if h.tool(name).Description == "" {
			t.Fatalf("tool %s has no description", name)
		}
	}
}

func TestAnAgentCreatesAndReadsARuleThroughTheTools(t *testing.T) {
	h := newAccessHub(t, map[string][]string{})
	made, err := h.invoke("agent", "taskmanager_rule_create", map[string]any{"kind": "rule", "title": "Tests first", "tags": []string{"process"}})
	if err != nil || made["code"] != "R-1" {
		t.Fatalf("got %v, %v", made, err)
	}
	read, err := h.invoke("agent", "taskmanager_rule_read", map[string]any{"ruleCode": "R-1"})
	if err != nil || read["title"] != "Tests first" {
		t.Fatalf("got %v, %v", read, err)
	}
}

func TestAFileLinkListCarriesAMissingFlagOnlyForAPathTheCheckerSaysIsGoneOverTheRoutes(t *testing.T) {
	f := newRuleFixtureWithChecker(t, pathsGone{"gone.go": true})
	h := f.h
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Thin routes"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules/R-1/files", map[string]any{"repo": "hub", "path": "gone.go"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules/R-1/files", map[string]any{"repo": "hub", "path": "here.go"})

	byPath := map[string]map[string]any{}
	for _, link := range f.list("ann", "/rules/R-1/files") {
		byPath[link["path"].(string)] = link
	}
	if byPath["gone.go"]["missing"] != true {
		t.Fatalf("gone path not flagged: %v", byPath)
	}
	if _, flagged := byPath["here.go"]["missing"]; flagged {
		t.Fatalf("present path flagged: %v", byPath)
	}
	h.expect(http.StatusForbidden, "nog", "GET", "/rules/R-1/files", nil)
}

func TestAFileLinkListHasNoMissingFlagWithoutACheckerOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	f.h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Thin routes"})
	f.h.expect(http.StatusCreated, "bob", "POST", "/rules/R-1/files", map[string]any{"repo": "hub", "path": "gone.go"})
	got := f.list("ann", "/rules/R-1/files")
	if _, flagged := got[0]["missing"]; len(got) != 1 || flagged {
		t.Fatalf("got %v", got)
	}
}

func TestADecisionListsTheRulesItProducedOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	other := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "another", "backlog": "docs", "step": "write"})["id"].(string)
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Body is Markdown"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Write Markdown"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "Unrelated"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Other decision"})
	h.expect(http.StatusNoContent, "bob", "POST", "/rules/D-1/links", map[string]any{"issue_id": f.issue, "relation": "decided_in"})
	h.expect(http.StatusNoContent, "bob", "POST", "/rules/R-1/links", map[string]any{"issue_id": f.issue, "relation": "decided_in"})
	h.expect(http.StatusNoContent, "bob", "POST", "/rules/R-2/links", map[string]any{"issue_id": other, "relation": "decided_in"})
	h.expect(http.StatusNoContent, "bob", "POST", "/rules/D-2/links", map[string]any{"issue_id": f.issue, "relation": "decided_in"})

	got := f.list("ann", "/rules/D-1/rules")
	if len(got) != 1 || got[0]["code"] != "R-1" {
		t.Fatalf("got %v", got)
	}
	h.expect(http.StatusBadRequest, "ann", "GET", "/rules/R-1/rules", nil)
	h.expect(http.StatusNotFound, "ann", "GET", "/rules/D-99/rules", nil)
	h.expect(http.StatusForbidden, "nog", "GET", "/rules/D-1/rules", nil)
}

func TestABacklogListsItsOpenQuestionsP1FirstOverTheRoutes(t *testing.T) {
	f := newRuleFixture(t)
	h := f.h
	h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Secret", "steps": []string{"Open"}})
	h.expect(http.StatusOK, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{"tags": []string{"docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "target_role": "Product owner", "title": "Plain", "tags": []string{"Docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "target_role": "Product owner", "title": "Urgent", "tags": []string{"docs"}, "priority": "P1"})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "target_role": "Product owner", "title": "Elsewhere", "tags": []string{"infra"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "question", "target_role": "Product owner", "title": "Settled", "tags": []string{"docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "decision", "title": "Answer", "tags": []string{"docs"}})
	h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": "rule", "title": "A rule", "tags": []string{"docs"}})
	h.expect(http.StatusOK, "bob", "PUT", "/rules/Q-4/answer", map[string]any{"decision_code": "D-1"})

	got := f.list("ann", "/backlogs/docs/questions")
	if len(got) != 2 || got[0]["code"] != "Q-2" || got[1]["code"] != "Q-1" {
		t.Fatalf("open, P1 first: %v", got)
	}
	if got := f.list("ann", "/backlogs/docs/questions?status=answered"); len(got) != 1 || got[0]["code"] != "Q-4" {
		t.Fatalf("answered %v", got)
	}
	h.expect(http.StatusBadRequest, "ann", "GET", "/backlogs/docs/questions?status=in_force", nil)
	h.expect(http.StatusBadRequest, "ann", "GET", "/backlogs/nope/questions", nil)
	h.expect(http.StatusForbidden, "ann", "GET", "/backlogs/secret/questions", nil)
	h.expect(http.StatusForbidden, "nog", "GET", "/backlogs/docs/questions", nil)
}

func TestTheNewRuleReadsAreNamedAfterTheirActions(t *testing.T) {
	h := newAccessHub(t, map[string][]string{})
	for _, name := range []string{"taskmanager_rule_produced_list", "taskmanager_backlog_questions_list"} {
		if h.tool(name).Description == "" {
			t.Fatalf("tool %s has no description", name)
		}
	}
}
