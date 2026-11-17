package routes_test

import (
	"fmt"
	"net/http"
	"testing"
)

type batchHub struct {
	*accessHub
}

func newBatchHub(t *testing.T) *batchHub {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"To do", "In progress", "Done"}})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	return &batchHub{accessHub: h}
}

func (h *batchHub) issue(title string) string {
	h.t.Helper()
	id := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": title})["id"].(string)
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": "software", "step": "to-do"})
	return id
}

func (h *batchHub) read(id string) map[string]any {
	h.t.Helper()
	return h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id, nil)
}

func patchOp(id string, patch map[string]any) map[string]any {
	return map[string]any{"op": "patch", "task_id": id, "patch": patch}
}

func moveOp(id, step string) map[string]any {
	return map[string]any{"op": "move_to_step", "task_id": id, "backlog": "software", "step": step}
}

func batchResult(body map[string]any, index int) map[string]any {
	return body["results"].([]any)[index].(map[string]any)
}

func TestBatchAppliesPatchesAndMovesInOrder(t *testing.T) {
	h := newBatchHub(t)
	a, b := h.issue("A"), h.issue("B")

	body := h.expect(http.StatusOK, "owner", "POST", "/batch", map[string]any{"operations": []any{
		patchOp(a, map[string]any{"title": "first", "priority": "high"}),
		patchOp(a, map[string]any{"title": "second"}),
		moveOp(a, "in-progress"),
		patchOp(b, map[string]any{"effort": "low"}),
	}})

	for i := 0; i < 4; i++ {
		if r := batchResult(body, i); r["ok"] != true || r["applied"] != true || r["index"] != float64(i) {
			t.Errorf("result %d = %v, want ok and applied", i, r)
		}
	}
	if body["applied"] != float64(4) || body["failed"] != float64(0) {
		t.Errorf("applied %v failed %v, want 4 and 0", body["applied"], body["failed"])
	}
	got := h.read(a)
	if got["title"] != "second" || got["priority"] != "high" || got["step"] != "in-progress" {
		t.Errorf("A = title %v priority %v step %v, want second, high, in-progress", got["title"], got["priority"], got["step"])
	}
	if h.read(b)["effort"] != "low" {
		t.Errorf("B effort %v, want low", h.read(b)["effort"])
	}
}

func TestBatchReportsARefusedOperationAndAppliesTheRest(t *testing.T) {
	h := newBatchHub(t)
	a, b, c := h.issue("A"), h.issue("B"), h.issue("C")

	body := h.expect(http.StatusOK, "owner", "POST", "/batch", map[string]any{"operations": []any{
		patchOp(a, map[string]any{"title": "A2"}),
		moveOp(b, "no-such-step"),
		patchOp(c, map[string]any{"title": "C2"}),
	}})

	if r := batchResult(body, 1); r["ok"] != false || r["applied"] != false || r["error"] == "" || r["error"] == nil {
		t.Errorf("result 1 = %v, want a refusal with its error", r)
	}
	if body["applied"] != float64(2) || body["failed"] != float64(1) {
		t.Errorf("applied %v failed %v, want 2 and 1", body["applied"], body["failed"])
	}
	if h.read(a)["title"] != "A2" || h.read(c)["title"] != "C2" || h.read(b)["step"] != "to-do" {
		t.Errorf("A %v, C %v, B step %v, want A2, C2, to-do", h.read(a)["title"], h.read(c)["title"], h.read(b)["step"])
	}
}

func TestBatchWithAllOrNothingAppliesNothingWhenOneIsRefused(t *testing.T) {
	h := newBatchHub(t)
	a, b, c := h.issue("A"), h.issue("B"), h.issue("C")

	body := h.expect(http.StatusOK, "owner", "POST", "/batch", map[string]any{"all_or_nothing": true, "operations": []any{
		patchOp(a, map[string]any{"title": "A2"}),
		moveOp(b, "no-such-step"),
		patchOp(c, map[string]any{"title": "C2"}),
	}})

	if body["applied"] != float64(0) || body["failed"] != float64(1) || body["all_or_nothing"] != true {
		t.Errorf("applied %v failed %v all_or_nothing %v, want 0, 1, true", body["applied"], body["failed"], body["all_or_nothing"])
	}
	if r := batchResult(body, 0); r["ok"] != true || r["applied"] != false {
		t.Errorf("result 0 = %v, want ok but not applied", r)
	}
	if r := batchResult(body, 1); r["ok"] != false || r["error"] == "" {
		t.Errorf("result 1 = %v, want the refusal", r)
	}
	if h.read(a)["title"] != "A" || h.read(c)["title"] != "C" {
		t.Errorf("A %v, C %v, want both untouched", h.read(a)["title"], h.read(c)["title"])
	}
}

func TestBatchWithAllOrNothingChecksLaterOperationsAgainstEarlierOnes(t *testing.T) {
	h := newBatchHub(t)
	a := h.issue("A")

	body := h.expect(http.StatusOK, "owner", "POST", "/batch", map[string]any{"all_or_nothing": true, "operations": []any{
		patchOp(a, map[string]any{"status": "cancelled"}),
		patchOp(a, map[string]any{"status": "in_progress"}),
	}})

	if r := batchResult(body, 0); r["ok"] != true || r["applied"] != false {
		t.Errorf("result 0 = %v, want ok but not applied", r)
	}
	if r := batchResult(body, 1); r["ok"] != false {
		t.Errorf("result 1 = %v, want refused: a cancelled issue cannot go back to in progress", r)
	}
	if body["applied"] != float64(0) || h.read(a)["status"] != "pending" {
		t.Errorf("applied %v, status %v, want nothing applied", body["applied"], h.read(a)["status"])
	}
}

func TestBatchWithAllOrNothingAppliesEverythingWhenAllPass(t *testing.T) {
	h := newBatchHub(t)
	a, b := h.issue("A"), h.issue("B")

	body := h.expect(http.StatusOK, "owner", "POST", "/batch", map[string]any{"all_or_nothing": true, "operations": []any{
		patchOp(a, map[string]any{"title": "A2"}),
		moveOp(b, "in-progress"),
	}})

	if body["applied"] != float64(2) || body["failed"] != float64(0) {
		t.Errorf("applied %v failed %v, want 2 and 0", body["applied"], body["failed"])
	}
	if h.read(a)["title"] != "A2" || h.read(b)["step"] != "in-progress" {
		t.Errorf("A %v, B step %v, want A2 and in-progress", h.read(a)["title"], h.read(b)["step"])
	}
}

func TestBatchRefusesMoreThanFiftyOperationsAndAnEmptyBatch(t *testing.T) {
	h := newBatchHub(t)
	a := h.issue("A")
	ops := make([]any, 51)
	for i := range ops {
		ops[i] = patchOp(a, map[string]any{"title": fmt.Sprintf("T%d", i)})
	}

	h.expect(http.StatusBadRequest, "owner", "POST", "/batch", map[string]any{"operations": ops})
	h.expect(http.StatusBadRequest, "owner", "POST", "/batch", map[string]any{"operations": []any{}})
	if h.read(a)["title"] != "A" {
		t.Errorf("title %v, want A: a refused batch changes nothing", h.read(a)["title"])
	}
	h.expect(http.StatusOK, "owner", "POST", "/batch", map[string]any{"operations": ops[:50]})
}

func TestBatchReportsABadOperationWithoutFailingTheBatch(t *testing.T) {
	h := newBatchHub(t)
	a := h.issue("A")

	body := h.expect(http.StatusOK, "owner", "POST", "/batch", map[string]any{"operations": []any{
		map[string]any{"op": "delete", "task_id": a},
		map[string]any{"op": "patch", "task_id": a},
		map[string]any{"op": "move_to_step", "task_id": a, "backlog": "software"},
		map[string]any{"op": "patch", "patch": map[string]any{"title": "x"}},
		patchOp("no-such-issue", map[string]any{"title": "x"}),
		patchOp(a, map[string]any{"title": "A2"}),
	}})

	for i := 0; i < 5; i++ {
		if r := batchResult(body, i); r["ok"] != false || r["error"] == "" {
			t.Errorf("result %d = %v, want a refusal", i, r)
		}
	}
	if r := batchResult(body, 5); r["ok"] != true {
		t.Errorf("result 5 = %v, want ok", r)
	}
	if h.read(a)["title"] != "A2" {
		t.Errorf("title %v, want A2", h.read(a)["title"])
	}
}

func TestBatchRefusesAnOperationOnABacklogTheCallerCannotMove(t *testing.T) {
	h := newBatchHub(t)
	a := h.issue("A")

	body := h.expect(http.StatusOK, "dev", "POST", "/batch", map[string]any{"operations": []any{
		patchOp(a, map[string]any{"title": "A2"}),
		moveOp(a, "in-progress"),
	}})

	for i := 0; i < 2; i++ {
		if r := batchResult(body, i); r["ok"] != false || r["applied"] != false {
			t.Errorf("result %d = %v, want refused for a viewer", i, r)
		}
	}
	if h.read(a)["title"] != "A" || h.read(a)["step"] != "to-do" {
		t.Errorf("A = %v %v, want untouched", h.read(a)["title"], h.read(a)["step"])
	}
}

func TestBatchWithAllOrNothingRefusesAViewerWithoutApplyingAnything(t *testing.T) {
	h := newBatchHub(t)
	a := h.issue("A")

	body := h.expect(http.StatusOK, "dev", "POST", "/batch", map[string]any{"all_or_nothing": true, "operations": []any{
		patchOp(a, map[string]any{"title": "A2"}),
	}})

	if body["applied"] != float64(0) || body["failed"] != float64(1) {
		t.Errorf("applied %v failed %v, want 0 and 1", body["applied"], body["failed"])
	}
}

func todoSpec(title string, waitsOn ...int) map[string]any {
	spec := map[string]any{"title": title, "description": "Makes " + title + ".", "instructions": "Do " + title + "."}
	if len(waitsOn) > 0 {
		spec["waits_on"] = waitsOn
	}
	return spec
}

func (h *batchHub) createWithTodos(want int, caller string, issue map[string]any, todos []any) map[string]any {
	h.t.Helper()
	body := map[string]any{"todos": todos}
	for k, v := range issue {
		body[k] = v
	}
	return h.expect(want, caller, "POST", "/create-with-todos", body)
}

func (h *batchHub) titles() []string {
	h.t.Helper()
	list := h.expect(http.StatusOK, "owner", "GET", "/pending-tasks?limit=100", nil)
	var titles []string
	for _, raw := range list["items"].([]any) {
		titles = append(titles, raw.(map[string]any)["title"].(string))
	}
	return titles
}

func TestCreateWithTodosMakesTheIssueAndAThreeTodoChain(t *testing.T) {
	h := newBatchHub(t)

	body := h.createWithTodos(http.StatusCreated, "owner", map[string]any{"title": "Chain", "description": "Done looks like: all three."}, []any{
		todoSpec("build"),
		todoSpec("mirror", 1),
		todoSpec("document", 2),
	})

	issue := body["issue"].(map[string]any)
	id := issue["id"].(string)
	if issue["title"] != "Chain" || issue["step"] == "" {
		t.Errorf("issue = %v, want the created issue", issue)
	}
	listed := h.expect(http.StatusOK, "owner", "GET", "/tasks/"+id+"/todos", nil)
	if listed["total"] != float64(3) {
		t.Fatalf("todos = %v, want 3", listed)
	}
	items := listed["items"].([]any)
	var ids []string
	for i, raw := range items {
		todo := raw.(map[string]any)
		ids = append(ids, todo["id"].(string))
		if todo["ordinality"] != float64(i+1) || todo["parent_task_id"] != id {
			t.Errorf("todo %d = %v, want ordinality %d under the issue", i, todo, i+1)
		}
		if i > 0 {
			deps := todo["depends_on"].([]any)
			if len(deps) != 1 || deps[0] != ids[i-1] {
				t.Errorf("todo %d depends_on %v, want [%s]", i, deps, ids[i-1])
			}
		}
	}
	if _, has := items[0].(map[string]any)["depends_on"]; has && len(items[0].(map[string]any)["depends_on"].([]any)) > 0 {
		t.Errorf("todo 1 depends_on %v, want none", items[0])
	}
	returned := body["todos"].([]any)
	if len(returned) != 3 || returned[2].(map[string]any)["id"] != ids[2] {
		t.Errorf("returned todos %v, want the three in order", returned)
	}
	if items[1].(map[string]any)["state"] != "Blocked" || items[0].(map[string]any)["state"] != "To do" {
		t.Errorf("states %v %v, want To do then Blocked", items[0].(map[string]any)["state"], items[1].(map[string]any)["state"])
	}
}

func TestCreateWithTodosTakesSeveralPredecessors(t *testing.T) {
	h := newBatchHub(t)

	body := h.createWithTodos(http.StatusCreated, "owner", map[string]any{"title": "Join"}, []any{
		todoSpec("left"), todoSpec("right"), todoSpec("join", 1, 2),
	})

	items := body["todos"].([]any)
	deps := items[2].(map[string]any)["depends_on"].([]any)
	if len(deps) != 2 {
		t.Errorf("join depends_on %v, want both predecessors", deps)
	}
}

func TestCreateWithTodosRefusesAForwardOrSelfReferenceAndCreatesNothing(t *testing.T) {
	cases := map[string][]any{
		"forward":      {todoSpec("one", 2), todoSpec("two")},
		"self":         {todoSpec("one", 1)},
		"cycle":        {todoSpec("one", 2), todoSpec("two", 1)},
		"out of range": {todoSpec("one"), todoSpec("two", 7)},
		"zero":         {todoSpec("one", 0)},
	}
	for name, todos := range cases {
		t.Run(name, func(t *testing.T) {
			h := newBatchHub(t)
			before := len(h.titles())

			h.createWithTodos(http.StatusBadRequest, "owner", map[string]any{"title": "Never made"}, todos)

			if got := len(h.titles()); got != before {
				t.Errorf("%d issues after, %d before: a refused call creates nothing", got, before)
			}
		})
	}
}

func TestCreateWithTodosRefusesATodoWithoutATitleOrInstructionsAndCreatesNothing(t *testing.T) {
	cases := map[string][]any{
		"no title":        {todoSpec("one"), map[string]any{"description": "d", "instructions": "i"}},
		"no instructions": {todoSpec("one"), map[string]any{"title": "two", "description": "d"}},
	}
	for name, todos := range cases {
		t.Run(name, func(t *testing.T) {
			h := newBatchHub(t)

			h.createWithTodos(http.StatusBadRequest, "owner", map[string]any{"title": "Never made"}, todos)

			for _, title := range h.titles() {
				if title == "Never made" {
					t.Errorf("the issue was made although a todo was refused")
				}
			}
		})
	}
}

func TestCreateWithTodosRefusesAnEmptyTodoList(t *testing.T) {
	h := newBatchHub(t)

	h.createWithTodos(http.StatusBadRequest, "owner", map[string]any{"title": "Empty"}, []any{})

	if len(h.titles()) != 0 {
		t.Errorf("issues %v, want none", h.titles())
	}
}

func TestCreateWithTodosRefusesACallerWhoCannotMoveOnTheBacklogAndCreatesNothing(t *testing.T) {
	h := newBatchHub(t)

	h.createWithTodos(http.StatusForbidden, "dev", map[string]any{"title": "Not allowed", "backlog": "software"}, []any{todoSpec("one")})

	if len(h.titles()) != 0 {
		t.Errorf("issues %v, want none", h.titles())
	}
}

func TestCreateWithTodosKeepsCanRunParallelAndTheIssueFields(t *testing.T) {
	h := newBatchHub(t)
	parallel := todoSpec("fast")
	parallel["can_run_parallel"] = true

	body := h.createWithTodos(http.StatusCreated, "owner", map[string]any{
		"title": "Fields", "priority": "high", "effort": "low", "tags": []string{"Ready"}, "files": []string{"code/a.go"}, "code": "SW901",
	}, []any{parallel, todoSpec("slow")})

	issue := body["issue"].(map[string]any)
	if issue["priority"] != "high" || issue["effort"] != "low" || issue["code"] != "SW901" {
		t.Errorf("issue = %v, want its fields kept", issue)
	}
	todos := body["todos"].([]any)
	if todos[0].(map[string]any)["can_run_parallel"] != true || todos[1].(map[string]any)["can_run_parallel"] != false {
		t.Errorf("todos = %v, want can_run_parallel kept per todo", todos)
	}
}
