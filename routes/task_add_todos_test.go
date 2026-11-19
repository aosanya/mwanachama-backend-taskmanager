package routes_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func (h *batchHub) addTodos(want int, caller, issue string, todos []any) map[string]any {
	h.t.Helper()
	return h.expect(want, caller, "POST", "/tasks/"+issue+"/add-todos", map[string]any{"todos": todos})
}
func (h *batchHub) todosOf(issue string) []map[string]any {
	h.t.Helper()
	var out []map[string]any
	for _, raw := range h.expect(http.StatusOK, "owner", "GET", "/tasks/"+issue+"/todos", nil)["items"].([]any) {
		out = append(out, raw.(map[string]any))
	}
	return out
}
func TestAddTodosWritesFourTodosOfAnExistingIssueInOneCallWithAChainAndAJoin(t *testing.T) {
	h := newBatchHub(t)
	issue := h.issue("Existing")
	body := h.addTodos(http.StatusCreated, "owner", issue, []any{todoSpec("build"), todoSpec("mirror", 1), todoSpec("test", 1), todoSpec("document", 2, 3)})
	added := body["todos"].([]any)
	if len(added) != 4 {
		t.Fatalf("reply %v, want four todos", body)
	}
	stored := h.todosOf(issue)
	if len(stored) != 4 {
		t.Fatalf("stored %v, want four todos", stored)
	}
	for i, raw := range added {
		got := raw.(map[string]any)
		if got["id"] != stored[i]["id"] || got["code"] != stored[i]["code"] || got["ordinality"] != float64(i+1) {
			t.Errorf("reply %d = %v, stored %v", i, got, stored[i])
		}
	}
	dependsOn := func(i int) []any {
		deps, _ := stored[i]["depends_on"].([]any)
		return deps
	}
	if len(dependsOn(0)) != 0 {
		t.Errorf("first depends_on %v, want none", dependsOn(0))
	}
	if deps := dependsOn(1); len(deps) != 1 || deps[0] != stored[0]["id"] {
		t.Errorf("second depends_on %v, want the first", deps)
	}
	if deps := dependsOn(2); len(deps) != 1 || deps[0] != stored[0]["id"] {
		t.Errorf("third depends_on %v, want the first", deps)
	}
	if deps := dependsOn(3); len(deps) != 2 || deps[0] != stored[1]["id"] || deps[1] != stored[2]["id"] {
		t.Errorf("fourth depends_on %v, want the second and the third", deps)
	}
	if stored[0]["state"] != "To do" || stored[3]["state"] != "Blocked" {
		t.Errorf("states %v and %v, want To do then Blocked", stored[0]["state"], stored[3]["state"])
	}
}
func TestAddTodosContinuesAfterTheTodosTheIssueAlreadyHas(t *testing.T) {
	h := newBatchHub(t)
	made := h.createWithTodos(http.StatusCreated, "owner", map[string]any{"title": "Has two"}, []any{todoSpec("first"), todoSpec("second", 1)})
	issue := made["issue"].(map[string]any)["id"].(string)
	body := h.addTodos(http.StatusCreated, "owner", issue, []any{todoSpec("third"), todoSpec("fourth", 1), todoSpec("fifth", 1, 2)})
	added := body["todos"].([]any)
	for i, raw := range added {
		if got := raw.(map[string]any)["ordinality"]; got != float64(3+i) {
			t.Errorf("added %d ordinality %v, want %d", i, got, 3+i)
		}
	}
	stored := h.todosOf(issue)
	if len(stored) != 5 {
		t.Fatalf("stored %v, want five", stored)
	}
	third, fourth, fifth := stored[2], stored[3], stored[4]
	if third["title"] != "third" || len(asList(third["depends_on"])) != 0 {
		t.Errorf("third = %v, want no predecessor", third)
	}
	if deps := asList(fourth["depends_on"]); len(deps) != 1 || deps[0] != third["id"] {
		t.Errorf("fourth depends_on %v, want the todo sent first, not the issue's first", deps)
	}
	if deps := asList(fifth["depends_on"]); len(deps) != 2 || deps[0] != third["id"] || deps[1] != fourth["id"] {
		t.Errorf("fifth depends_on %v, want the first and second sent", deps)
	}
}
func asList(v any) []any {
	list, _ := v.([]any)
	return list
}
func TestAddTodosKeepsCanRunParallelAndTheDescription(t *testing.T) {
	h := newBatchHub(t)
	issue := h.issue("Existing")
	parallel := todoSpec("fast")
	parallel["can_run_parallel"] = true
	h.addTodos(http.StatusCreated, "owner", issue, []any{parallel, todoSpec("slow")})
	stored := h.todosOf(issue)
	if stored[0]["can_run_parallel"] != true || stored[1]["can_run_parallel"] != false || stored[0]["description"] != "Makes fast." {
		t.Errorf("stored %v, want can_run_parallel and description kept", stored)
	}
}
func TestAddTodosRefusesAForwardOrSelfReferenceAndCreatesNothing(t *testing.T) {
	cases := map[string][]any{"forward": {todoSpec("one", 2), todoSpec("two")}, "self": {todoSpec("one", 1)}, "cycle": {todoSpec("one", 2), todoSpec("two", 1)}, "out of range": {todoSpec("one"), todoSpec("two", 7)}, "zero": {todoSpec("one", 0)}, "late": {todoSpec("one"), todoSpec("two"), todoSpec("three", 3)}}
	for name, todos := range cases {
		t.Run(name, func(t *testing.T) {
			h := newBatchHub(t)
			issue := h.issue("Existing")
			h.addTodos(http.StatusCreated, "owner", issue, []any{todoSpec("kept")})
			h.addTodos(http.StatusBadRequest, "owner", issue, todos)
			if stored := h.todosOf(issue); len(stored) != 1 || stored[0]["title"] != "kept" {
				t.Errorf("todos %v, want only the one written before", stored)
			}
		})
	}
}
func TestAddTodosRefusesATodoWithoutATitleOrInstructionsAndCreatesNothing(t *testing.T) {
	cases := map[string][]any{"no title": {todoSpec("one"), map[string]any{"description": "d", "instructions": "i"}}, "no instructions": {todoSpec("one"), map[string]any{"title": "two", "description": "d"}}}
	for name, todos := range cases {
		t.Run(name, func(t *testing.T) {
			h := newBatchHub(t)
			issue := h.issue("Existing")
			h.addTodos(http.StatusBadRequest, "owner", issue, todos)
			if stored := h.todosOf(issue); len(stored) != 0 {
				t.Errorf("todos %v, want none", stored)
			}
		})
	}
}
func TestAddTodosRefusesAnEmptyTodoList(t *testing.T) {
	h := newBatchHub(t)
	issue := h.issue("Existing")
	h.addTodos(http.StatusBadRequest, "owner", issue, []any{})
	h.expect(http.StatusBadRequest, "owner", "POST", "/tasks/"+issue+"/add-todos", map[string]any{})
	if stored := h.todosOf(issue); len(stored) != 0 {
		t.Errorf("todos %v, want none", stored)
	}
}
func TestAddTodosRefusesAnIssueThatDoesNotExist(t *testing.T) {
	h := newBatchHub(t)
	h.addTodos(http.StatusNotFound, "owner", "no-such-issue", []any{todoSpec("one")})
}
func TestAddTodosRefusesACallerWhoCannotMoveOnTheBacklogAndCreatesNothing(t *testing.T) {
	h := newBatchHub(t)
	issue := h.issue("Existing")
	h.addTodos(http.StatusForbidden, "dev", issue, []any{todoSpec("one")})
	if code, _ := h.call("nobody", "POST", "/tasks/"+issue+"/add-todos", map[string]any{"todos": []any{todoSpec("one")}}); code != http.StatusForbidden && code != http.StatusNotFound {
		t.Errorf("a person in no group got %d, want 403 or 404", code)
	}
	if stored := h.todosOf(issue); len(stored) != 0 {
		t.Errorf("todos %v, want none", stored)
	}
}
func TestAddTodosToolIsNamedAndTheReplyIsCompact(t *testing.T) {
	h := newBatchHub(t)
	issue := h.issue("Existing")
	todos := []any{todoSpec("one"), todoSpec("two", 1), todoSpec("three", 2), todoSpec("four", 1, 3)}
	got, err := h.invoke("owner", "taskmanager_task_add_todos", map[string]any{"taskID": issue, "todos": todos})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("reply keys %v, want only todos", got)
	}
	items := got["todos"].([]any)
	if len(items) != 4 {
		t.Fatalf("reply %v, want four todos", got)
	}
	for i, raw := range items {
		item := raw.(map[string]any)
		if len(item) != 3 || item["id"] == "" || item["code"] == "" || item["ordinality"] != float64(i+1) {
			t.Errorf("item %d = %v, want only id, code and ordinality", i, item)
		}
	}
	encoded, _ := json.Marshal(got)
	if len(encoded) > 500 {
		t.Errorf("reply is %d bytes, want a compact one", len(encoded))
	}
	if stored := h.todosOf(issue); len(stored) != 4 {
		t.Errorf("stored %v, want four", stored)
	}
}
