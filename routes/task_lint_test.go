package routes_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"
)

type lintHub struct {
	*accessHub
	rank int
}

func newLintHub(t *testing.T) *lintHub {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"To do", "In progress", "Done"}})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	return &lintHub{accessHub: h}
}

func wellFiledTask(code string) map[string]any {
	return map[string]any{
		"title":       "Title " + code,
		"code":        code,
		"description": "Makes the thing and puts it in one place.\nDone looks like: the tests pass.",
		"files":       []string{"code/" + code + ".go"},
		"effort":      "low",
		"priority":    "high",
	}
}

func wellFiledTodos() []map[string]any {
	return []map[string]any{
		{"title": "Build the thing", "description": "Makes the thing; output in one file; done when tested."},
		{"title": "Documentation: one table row", "description": "A few lines in the tools page."},
	}
}

func (h *lintHub) file(code string, task map[string]any, todos []map[string]any) string {
	h.t.Helper()
	h.rank++
	task["rank"] = h.rank * 100
	id := h.expect(http.StatusCreated, "owner", "POST", "/tasks", task)["id"].(string)
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": "software", "step": "to-do"})
	for _, todo := range todos {
		todo["instructions"] = "do it"
		todo["parent_task_id"] = id
		h.expect(http.StatusCreated, "owner", "POST", "/todos", todo)
	}
	return id
}

func lintRules(body map[string]any, index int) string {
	rules := []string{}
	item := body["items"].([]any)[index].(map[string]any)
	for _, raw := range item["findings"].([]any) {
		rules = append(rules, raw.(map[string]any)["rule"].(string))
	}
	sort.Strings(rules)
	return strings.Join(rules, ",")
}

func TestLintAWellFiledIssueIsClean(t *testing.T) {
	h := newLintHub(t)
	id := h.file("OK1", wellFiledTask("OK1"), wellFiledTodos())

	body := h.expect(http.StatusOK, "owner", "GET", "/lint?taskID="+id, nil)

	if got := lintRules(body, 0); got != "" {
		t.Errorf("findings %q, want none (%v)", got, body)
	}
	if body["clean"] != float64(1) || body["total"] != float64(1) {
		t.Errorf("clean %v total %v, want 1 and 1", body["clean"], body["total"])
	}
}

func TestLintEachRuleFiresOnAnIssueBuiltToBreakOnlyIt(t *testing.T) {
	without := func(key string) func(map[string]any) {
		return func(task map[string]any) { delete(task, key) }
	}
	cases := []struct {
		rule  string
		edit  func(map[string]any)
		todos []map[string]any
	}{
		{"no_description", func(task map[string]any) { task["description"] = "" }, wellFiledTodos()},
		{"no_done_when", func(task map[string]any) { task["description"] = "Makes the thing." }, wellFiledTodos()},
		{"no_todos", func(map[string]any) {}, nil},
		{"last_todo_not_documentation", func(map[string]any) {}, []map[string]any{
			{"title": "Document the thing", "description": "Lines."},
			{"title": "Build the thing", "description": "Makes the thing."},
		}},
		{"no_files", without("files"), wellFiledTodos()},
		{"effort_unknown", without("effort"), wellFiledTodos()},
		{"todo_without_description", func(map[string]any) {}, []map[string]any{
			{"title": "Build the thing"},
			{"title": "Documentation: one table row", "description": "A few lines."},
		}},
	}
	for _, c := range cases {
		t.Run(c.rule, func(t *testing.T) {
			h := newLintHub(t)
			task := wellFiledTask("R1")
			c.edit(task)
			id := h.file("R1", task, c.todos)

			body := h.expect(http.StatusOK, "owner", "GET", "/lint?taskID="+id, nil)

			if got := lintRules(body, 0); got != c.rule {
				t.Errorf("findings %q, want only %q (%v)", got, c.rule, body)
			}
			if body["clean"] != float64(0) {
				t.Errorf("clean %v, want 0", body["clean"])
			}
		})
	}
}

func TestLintFindingsCarryAMessage(t *testing.T) {
	h := newLintHub(t)
	task := wellFiledTask("M1")
	delete(task, "files")
	id := h.file("M1", task, wellFiledTodos())

	body := h.expect(http.StatusOK, "owner", "GET", "/lint?taskID="+id, nil)

	item := body["items"].([]any)[0].(map[string]any)
	finding := item["findings"].([]any)[0].(map[string]any)
	if strings.TrimSpace(finding["message"].(string)) == "" {
		t.Errorf("a finding has no message: %v", finding)
	}
	for _, field := range []string{"id", "code", "title", "findings"} {
		if _, has := item[field]; !has {
			t.Errorf("an item lacks %q: %v", field, item)
		}
	}
}

func TestLintAnUnknownIssueIsNotFound(t *testing.T) {
	h := newLintHub(t)

	h.expect(http.StatusNotFound, "owner", "GET", "/lint?taskID=no-such-issue", nil)
}

func TestLintAnIssueTheCallerCannotSeeIsNotFound(t *testing.T) {
	h := newLintHub(t)
	id := h.file("V1", wellFiledTask("V1"), wellFiledTodos())

	h.expect(http.StatusNotFound, "nobody", "GET", "/lint?taskID="+id, nil)
}

func TestLintReadsABacklogStepAndPagesIt(t *testing.T) {
	h := newLintHub(t)
	for _, code := range []string{"P1", "P2", "P3"} {
		h.file(code, wellFiledTask(code), nil)
	}

	first := h.expect(http.StatusOK, "owner", "GET", "/lint?backlog=software&step=to-do&limit=2", nil)
	second := h.expect(http.StatusOK, "owner", "GET", "/lint?backlog=software&step=to-do&limit=2&offset=2", nil)

	if len(first["items"].([]any)) != 2 || first["total"] != float64(3) || first["clean"] != float64(0) {
		t.Errorf("first page %v, want 2 items, total 3, clean 0", first)
	}
	if len(second["items"].([]any)) != 1 || second["total"] != float64(3) {
		t.Errorf("second page %v, want 1 item, total 3", second)
	}
	firstCode := first["items"].([]any)[0].(map[string]any)["code"]
	lastCode := second["items"].([]any)[0].(map[string]any)["code"]
	if firstCode != "P1" || lastCode != "P3" {
		t.Errorf("pages start at %v and %v, want P1 and P3", firstCode, lastCode)
	}
}

func TestLintCountsCleanIssuesOnThePage(t *testing.T) {
	h := newLintHub(t)
	h.file("C1", wellFiledTask("C1"), wellFiledTodos())
	h.file("C2", wellFiledTask("C2"), nil)

	body := h.expect(http.StatusOK, "owner", "GET", "/lint?backlog=software&step=to-do", nil)

	if body["clean"] != float64(1) || body["total"] != float64(2) {
		t.Errorf("clean %v total %v, want 1 and 2", body["clean"], body["total"])
	}
}

func TestLintLimitAboveTheMaximumIsRefused(t *testing.T) {
	h := newLintHub(t)

	h.expect(http.StatusBadRequest, "owner", "GET", "/lint?backlog=software&limit=201", nil)
	h.expect(http.StatusOK, "owner", "GET", "/lint?backlog=software&limit=200", nil)
}

func TestLintANegativeOffsetIsRefused(t *testing.T) {
	h := newLintHub(t)

	h.expect(http.StatusBadRequest, "owner", "GET", "/lint?backlog=software&offset=-1", nil)
}

func TestLintAnUnseenBacklogIsRefused(t *testing.T) {
	h := newLintHub(t)

	h.expect(http.StatusBadRequest, "nobody", "GET", "/lint?backlog=software", nil)
}
