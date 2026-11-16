package routes_test

import (
	"net/http"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func (f *moreWritesFixture) todo(t *testing.T, issue, title string, waitsOn ...string) string {
	t.Helper()
	body := map[string]any{"title": title, "instructions": "do " + title, "parent_task_id": issue}
	if len(waitsOn) > 0 {
		body["depends_on"] = waitsOn
	}
	return f.h.expect(http.StatusCreated, "boss", "POST", "/todos", body)["id"].(string)
}

func (f *moreWritesFixture) otherIssue() string {
	return f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "another issue", "backlog": f.docs, "step": "write"})["id"].(string)
}

func todoTitles(list map[string]any) []string {
	var titles []string
	for _, raw := range list["items"].([]any) {
		titles = append(titles, raw.(map[string]any)["title"].(string))
	}
	return titles
}

func todoNamed(t *testing.T, list map[string]any, title string) map[string]any {
	t.Helper()
	for _, raw := range list["items"].([]any) {
		if item := raw.(map[string]any); item["title"] == title {
			return item
		}
	}
	t.Fatalf("no todo %q in %v", title, list)
	return nil
}

func sameTitles(got []string, want ...string) bool {
	return strings.Join(got, "|") == strings.Join(want, "|")
}

func TestAnIssueWithNoTodosListsNoneAndCountsZeroOfZero(t *testing.T) {
	f := newMoreWritesFixture(t)

	list := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/todos", nil)

	if len(list["items"].([]any)) != 0 || list["done"] != float64(0) || list["total"] != float64(0) {
		t.Fatalf("got %v", list)
	}
}

func TestTheTodosOfAnIssueAreListedInOrderWithStateAndDoneOverTotal(t *testing.T) {
	f := newMoreWritesFixture(t)
	first := f.todo(t, f.issue, "first")
	f.todo(t, f.issue, "second", first)
	f.todo(t, f.issue, "third")

	list := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/todos", nil)

	if !sameTitles(todoTitles(list), "first", "second", "third") || list["done"] != float64(0) || list["total"] != float64(3) {
		t.Fatalf("got %v", list)
	}
	if todoNamed(t, list, "first")["state"] != "To do" || todoNamed(t, list, "first")["status"] != "pending" {
		t.Fatalf("first %v", todoNamed(t, list, "first"))
	}
	second := todoNamed(t, list, "second")
	if second["state"] != "Blocked" || second["status"] != "blocked" {
		t.Fatalf("second waits on first and reads Blocked: %v", second)
	}
	if ids, _ := second["depends_on"].([]any); len(ids) != 1 || ids[0] != first {
		t.Fatalf("depends_on %v, want [%s]", second["depends_on"], first)
	}
}

func TestATodoThatWaitsReadsBlockedAndTurnsToDoWhenEveryPredecessorIsDone(t *testing.T) {
	f := newMoreWritesFixture(t)
	a := f.todo(t, f.issue, "a")
	b := f.todo(t, f.issue, "b")
	f.todo(t, f.issue, "c", a, b)
	path := "/tasks/" + f.issue + "/todos"

	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+a+"/status", map[string]any{"status": "Done"})
	if got := todoNamed(t, f.h.expect(http.StatusOK, "boss", "GET", path, nil), "c"); got["state"] != "Blocked" {
		t.Fatalf("c still waits on b: %v", got)
	}
	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+b+"/status", map[string]any{"status": "Done"})

	list := f.h.expect(http.StatusOK, "boss", "GET", path, nil)
	if got := todoNamed(t, list, "c"); got["state"] != "To do" || got["status"] != "pending" {
		t.Fatalf("c %v", got)
	}
	if list["done"] != float64(2) || list["total"] != float64(3) {
		t.Fatalf("got %v", list)
	}
}

func TestTheFourStatusesAreSetByNameAndByApiValue(t *testing.T) {
	f := newMoreWritesFixture(t)
	id := f.todo(t, f.issue, "piece")
	cases := []struct{ sent, state, status string }{
		{"In progress", "In progress", "dispatched"},
		{"Done", "Done", "completed"},
		{"To do", "To do", "pending"},
		{"dispatched", "In progress", "dispatched"},
		{"completed", "Done", "completed"},
		{"pending", "To do", "pending"},
	}
	for _, c := range cases {
		got := f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+id+"/status", map[string]any{"status": c.sent})
		if got["state"] != c.state || got["status"] != c.status {
			t.Fatalf("%s: got %v", c.sent, got)
		}
	}
}

func TestBlockingByHandNeedsAReasonAndLiftingItClearsTheReason(t *testing.T) {
	f := newMoreWritesFixture(t)
	id := f.todo(t, f.issue, "piece")
	status := "/todos/" + id + "/status"

	refused := f.h.expect(http.StatusBadRequest, "boss", "PUT", status, map[string]any{"status": "Blocked"})
	if !strings.Contains(strings.ToLower(refused["error"].(string)), "reason") {
		t.Fatalf("got %v", refused)
	}
	f.h.expect(http.StatusBadRequest, "boss", "PUT", status, map[string]any{"status": "Blocked", "blocked_reason": "  "})

	blocked := f.h.expect(http.StatusOK, "boss", "PUT", status, map[string]any{"status": "Blocked", "blocked_reason": "waiting for the key"})
	if blocked["state"] != "Blocked" || blocked["blocked_reason"] != "waiting for the key" {
		t.Fatalf("got %v", blocked)
	}
	listed := todoNamed(t, f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/todos", nil), "piece")
	if listed["blocked_reason"] != "waiting for the key" {
		t.Fatalf("got %v", listed)
	}

	lifted := f.h.expect(http.StatusOK, "boss", "PUT", status, map[string]any{"status": "To do", "blocked_reason": "ignored"})
	if lifted["state"] != "To do" || lifted["blocked_reason"] != nil && lifted["blocked_reason"] != "" {
		t.Fatalf("got %v", lifted)
	}
}

func TestAnUnknownStatusIsRefusedAndTheTodoKeepsItsStatus(t *testing.T) {
	f := newMoreWritesFixture(t)
	id := f.todo(t, f.issue, "piece")

	refused := f.h.expect(http.StatusBadRequest, "boss", "PUT", "/todos/"+id+"/status", map[string]any{"status": "Finished"})
	if !strings.Contains(refused["error"].(string), "To do, In progress, Done or Blocked") {
		t.Fatalf("got %v", refused)
	}
	f.h.expect(http.StatusBadRequest, "boss", "PUT", "/todos/"+id+"/status", map[string]any{"status": "failed"})
	f.h.expect(http.StatusNotFound, "boss", "PUT", "/todos/nope/status", map[string]any{"status": "Done"})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/todos/"+id, nil); got["status"] != "pending" {
		t.Fatalf("got %v", got)
	}
}

func TestATodoTitleInstructionsAndWaitsOnAreEditedAndOtherFieldsStay(t *testing.T) {
	f := newMoreWritesFixture(t)
	a := f.todo(t, f.issue, "a")
	b := f.todo(t, f.issue, "b")

	edited := f.h.expect(http.StatusOK, "boss", "PATCH", "/todos/"+b, map[string]any{"title": "b renamed", "instructions": "new words", "depends_on": []string{a}})
	if edited["title"] != "b renamed" || edited["instructions"] != "new words" || edited["state"] != "Blocked" {
		t.Fatalf("got %v", edited)
	}
	if ids, _ := edited["depends_on"].([]any); len(ids) != 1 || ids[0] != a {
		t.Fatalf("depends_on %v", edited["depends_on"])
	}

	renamedAgain := f.h.expect(http.StatusOK, "boss", "PATCH", "/todos/"+b, map[string]any{"title": "b again"})
	if renamedAgain["title"] != "b again" || renamedAgain["instructions"] != "new words" {
		t.Fatalf("got %v", renamedAgain)
	}
	if ids, _ := renamedAgain["depends_on"].([]any); len(ids) != 1 {
		t.Fatalf("a patch without depends_on left it as it was: %v", renamedAgain)
	}

	cleared := f.h.expect(http.StatusOK, "boss", "PATCH", "/todos/"+b, map[string]any{"depends_on": []string{}})
	if ids, _ := cleared["depends_on"].([]any); len(ids) != 0 || cleared["state"] != "To do" {
		t.Fatalf("got %v", cleared)
	}
}

func TestAnEditThatMakesACycleIsRefusedWithItsMessageAndNothingChanges(t *testing.T) {
	f := newMoreWritesFixture(t)
	a := f.todo(t, f.issue, "a")
	b := f.todo(t, f.issue, "b", a)

	refused := f.h.expect(http.StatusConflict, "boss", "PATCH", "/todos/"+a, map[string]any{"title": "a changed", "depends_on": []string{b}})
	if !strings.Contains(refused["error"].(string), mwanachamataskmanager.ErrTaskTodoCycle.Error()) {
		t.Fatalf("got %v", refused)
	}
	f.h.expect(http.StatusConflict, "boss", "PATCH", "/todos/"+a, map[string]any{"depends_on": []string{a}})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/todos/"+a, nil); got["title"] != "a" {
		t.Fatalf("a refused edit changed the todo: %v", got)
	}
}

func TestATodoCannotWaitOnATodoOfAnotherIssueOrOnOneThatIsNotThere(t *testing.T) {
	f := newMoreWritesFixture(t)
	mine := f.todo(t, f.issue, "mine")
	theirs := f.todo(t, f.otherIssue(), "theirs")

	refused := f.h.expect(http.StatusBadRequest, "boss", "PATCH", "/todos/"+mine, map[string]any{"depends_on": []string{theirs}})
	if !strings.Contains(refused["error"].(string), mwanachamataskmanager.ErrTaskTodoCrossIssue.Error()) {
		t.Fatalf("got %v", refused)
	}
	f.h.expect(http.StatusBadRequest, "boss", "PATCH", "/todos/"+mine, map[string]any{"depends_on": []string{"nope"}})
	f.h.expect(http.StatusBadRequest, "boss", "PATCH", "/todos/"+mine, map[string]any{"code": "TD-9"})
	f.h.expect(http.StatusNotFound, "boss", "PATCH", "/todos/nope", map[string]any{"title": "x"})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/todos/"+mine, nil); got["depends_on"] != nil {
		t.Fatalf("a refused edit changed the todo: %v", got)
	}
}

func TestTodosAreReorderedAndTheListFollows(t *testing.T) {
	f := newMoreWritesFixture(t)
	a := f.todo(t, f.issue, "a")
	b := f.todo(t, f.issue, "b")
	c := f.todo(t, f.issue, "c")

	reordered := f.h.expect(http.StatusOK, "boss", "PUT", "/tasks/"+f.issue+"/todos/order", map[string]any{"ids": []string{c, a, b}})
	if !sameTitles(todoTitles(reordered), "c", "a", "b") {
		t.Fatalf("got %v", reordered)
	}
	if listed := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/todos", nil); !sameTitles(todoTitles(listed), "c", "a", "b") {
		t.Fatalf("got %v", listed)
	}
}

func TestAReorderThatMissesATodoOrNamesAnotherIssuesTodoIsRefusedAndTheOrderStays(t *testing.T) {
	f := newMoreWritesFixture(t)
	a := f.todo(t, f.issue, "a")
	b := f.todo(t, f.issue, "b")
	theirs := f.todo(t, f.otherIssue(), "theirs")
	order := "/tasks/" + f.issue + "/todos/order"

	refused := f.h.expect(http.StatusConflict, "boss", "PUT", order, map[string]any{"ids": []string{b}})
	if !strings.Contains(refused["error"].(string), mwanachamataskmanager.ErrTaskTodoOrderMismatch.Error()) {
		t.Fatalf("got %v", refused)
	}
	f.h.expect(http.StatusConflict, "boss", "PUT", order, map[string]any{"ids": []string{b, a, theirs}})
	f.h.expect(http.StatusConflict, "boss", "PUT", order, map[string]any{"ids": []string{a, a}})
	if listed := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/todos", nil); !sameTitles(todoTitles(listed), "a", "b") {
		t.Fatalf("got %v", listed)
	}
}

func TestADeletedTodoLeavesTheListAndTheCountsAndNothingWaitsOnItAnymore(t *testing.T) {
	f := newMoreWritesFixture(t)
	a := f.todo(t, f.issue, "a")
	f.todo(t, f.issue, "b", a)

	f.h.expect(http.StatusNoContent, "boss", "DELETE", "/todos/"+a, nil)

	list := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/todos", nil)
	if !sameTitles(todoTitles(list), "b") || list["total"] != float64(1) {
		t.Fatalf("got %v", list)
	}
	if b := todoNamed(t, list, "b"); b["state"] != "To do" || b["depends_on"] != nil {
		t.Fatalf("b %v", b)
	}
	f.h.expect(http.StatusNotFound, "boss", "DELETE", "/todos/"+a, nil)
	f.h.expect(http.StatusNotFound, "boss", "PATCH", "/todos/"+a, map[string]any{"title": "x"})
	f.h.expect(http.StatusNotFound, "boss", "PUT", "/todos/"+a+"/status", map[string]any{"status": "Done"})
}

func TestTheCountsOfManyIssuesComeInOneReadAndLeaveOutIssuesWithNoTodos(t *testing.T) {
	f := newMoreWritesFixture(t)
	second := f.otherIssue()
	empty := f.otherIssue()
	a := f.todo(t, f.issue, "a")
	f.todo(t, f.issue, "b")
	f.todo(t, second, "c")
	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+a+"/status", map[string]any{"status": "Done"})

	got := f.h.expect(http.StatusOK, "boss", "GET", "/todo-counts?task_id="+f.issue+"&task_id="+second+"&task_id="+empty+"&task_id=nope", nil)

	items := got["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("got %v", got)
	}
	first, other := items[0].(map[string]any), items[1].(map[string]any)
	if first["task_id"] != f.issue || first["done"] != float64(1) || first["total"] != float64(2) ||
		other["task_id"] != second || other["done"] != float64(0) || other["total"] != float64(1) {
		t.Fatalf("got %v", got)
	}
	f.h.expect(http.StatusBadRequest, "boss", "GET", "/todo-counts", nil)
}

func TestAReaderSeesTheListAndTheCountsButCannotChangeATodo(t *testing.T) {
	f := newMoreWritesFixture(t)
	a := f.todo(t, f.issue, "a")
	b := f.todo(t, f.issue, "b")

	list := f.h.expect(http.StatusOK, "ann", "GET", "/tasks/"+f.issue+"/todos", nil)
	if !sameTitles(todoTitles(list), "a", "b") {
		t.Fatalf("got %v", list)
	}
	counts := f.h.expect(http.StatusOK, "ann", "GET", "/todo-counts?task_id="+f.issue, nil)
	if len(counts["items"].([]any)) != 1 {
		t.Fatalf("got %v", counts)
	}
	f.h.expect(http.StatusForbidden, "ann", "PATCH", "/todos/"+a, map[string]any{"title": "x"})
	f.h.expect(http.StatusForbidden, "ann", "PUT", "/todos/"+a+"/status", map[string]any{"status": "Done"})
	f.h.expect(http.StatusForbidden, "ann", "DELETE", "/todos/"+a, nil)
	f.h.expect(http.StatusForbidden, "ann", "PUT", "/tasks/"+f.issue+"/todos/order", map[string]any{"ids": []string{b, a}})
	if after := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/todos", nil); !sameTitles(todoTitles(after), "a", "b") || todoNamed(t, after, "a")["state"] != "To do" {
		t.Fatalf("a refused write changed the list: %v", after)
	}
}

func TestAPersonWhoCannotSeeTheIssueCannotReadOrChangeItsTodos(t *testing.T) {
	f := newMoreWritesFixture(t)
	a := f.todo(t, f.issue, "a")

	f.h.expect(http.StatusNotFound, "nog", "GET", "/tasks/"+f.issue+"/todos", nil)
	f.h.expect(http.StatusNotFound, "nog", "PATCH", "/todos/"+a, map[string]any{"title": "x"})
	f.h.expect(http.StatusNotFound, "nog", "PUT", "/todos/"+a+"/status", map[string]any{"status": "Done"})
	f.h.expect(http.StatusNotFound, "nog", "DELETE", "/todos/"+a, nil)
	f.h.expect(http.StatusNotFound, "nog", "PUT", "/tasks/"+f.issue+"/todos/order", map[string]any{"ids": []string{a}})
	if counts := f.h.expect(http.StatusOK, "nog", "GET", "/todo-counts?task_id="+f.issue, nil); len(counts["items"].([]any)) != 0 {
		t.Fatalf("got %v", counts)
	}
}

func TestTheTodoToolsAreDeclaredWithTheirActionsAndTheirErrorsHaveStatuses(t *testing.T) {
	built, err := routes.Build(mwanachamataskmanager.Unbound())
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, r := range built {
		have[r.Method+" "+r.Path+" "+r.Action] = true
	}
	for _, want := range []string{
		"GET /tasks/{taskID}/todos taskmanager.task_todo.list_for_issue",
		"GET /todo-counts taskmanager.task_todo.count_for_issues",
		"PATCH /todos/{todoID} taskmanager.task_todo.update",
		"PUT /tasks/{taskID}/todos/order taskmanager.task_todo.reorder",
		"DELETE /todos/{todoID} taskmanager.task_todo.delete",
		"PUT /todos/{todoID}/status taskmanager.task_todo.set_status",
	} {
		if !have[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestTheLastTodoSetToDoneMovesAnInProgressIssueToReviewAndTheLogSaysWhy(t *testing.T) {
	f := newMoreWritesFixture(t)
	lists := f.h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Issue todo lists", "steps": []string{"To do", "In progress", "Review", "Done"}})["id"].(string)
	issue := f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "an issue with todos", "backlog": lists, "step": "in-progress"})["id"].(string)
	first := f.todo(t, issue, "first")
	second := f.todo(t, issue, "second")

	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+first+"/status", map[string]any{"status": "Done"})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+issue, nil)["step"]; got != "in-progress" {
		t.Fatalf("with a piece open the issue is in %v", got)
	}
	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+second+"/status", map[string]any{"status": "Done"})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+issue, nil)["step"]; got != "review" {
		t.Fatalf("after the last piece the issue is in %v", got)
	}
	log := f.h.notesAs("boss", "/tasks/"+issue+"/notes")
	if len(log) != 1 || log[0]["kind"] != "worker" || log[0]["text"] != "Every todo is Done: moved to Review" {
		t.Fatalf("got %v", log)
	}

	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+second+"/status", map[string]any{"status": "To do"})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+issue, nil)["step"]; got != "review" {
		t.Fatalf("reopening a piece moved the issue to %v", got)
	}
}
