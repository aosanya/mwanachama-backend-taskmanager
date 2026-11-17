package routes_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

type pendingTodosFixture struct {
	h      *accessHub
	issues map[string]string
}

func newPendingTodosFixture(t *testing.T) *pendingTodosFixture {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Book", "steps": []string{"Open", "In progress", "Done"}})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Elsewhere", "steps": []string{"Open", "In progress"}})
	f := &pendingTodosFixture{h: h, issues: map[string]string{}}
	f.issue("A", "book", "in-progress", true)
	f.issue("B", "book", "in-progress", true)
	f.issue("C", "book", "open", false)
	f.issue("D", "book", "in-progress", true)
	f.issue("E", "elsewhere", "in-progress", true)
	f.issue("F", "book", "open", false)

	f.todos("A", "a-done", "a-todo", "a-blocked")
	f.setState("a-done", "Done")
	f.setState("a-blocked", "Blocked", "waits on the owner")
	f.todos("B", "b-done", "b-blocked", "b-todo")
	f.setState("b-done", "Done")
	f.setState("b-blocked", "Blocked", "waits on a decision")
	f.todos("C", "c-todo")
	f.todos("D", "d-done")
	f.setState("d-done", "Done")
	f.todos("E", "e-todo")
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/book/access/g-dev", map[string]string{"level": "see"})
	return f
}

func (f *pendingTodosFixture) issue(code, backlog, step string, inProgress bool) {
	f.h.t.Helper()
	made := f.h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "issue " + code, "code": code, "backlog": backlog, "step": step})
	id := made["id"].(string)
	f.issues[code] = id
	if inProgress {
		f.h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"status": "in_progress"})
	}
}

func (f *pendingTodosFixture) todos(issueCode string, titles ...string) {
	f.h.t.Helper()
	for _, title := range titles {
		made := f.h.expect(http.StatusCreated, "owner", "POST", "/todos", map[string]any{"title": title, "instructions": "do " + title, "parent_task_id": f.issues[issueCode]})
		f.issues[title] = made["id"].(string)
	}
}

func (f *pendingTodosFixture) setState(title, state string, reason ...string) {
	f.h.t.Helper()
	body := map[string]any{"status": state}
	if len(reason) > 0 {
		body["blocked_reason"] = reason[0]
	}
	f.h.expect(http.StatusOK, "owner", "PUT", "/todos/"+f.issues[title]+"/status", body)
}

func pendingTodoCodes(body map[string]any) []string {
	var codes []string
	for _, raw := range body["items"].([]any) {
		codes = append(codes, raw.(map[string]any)["code"].(string))
	}
	return codes
}

func pendingTodoTitles(item map[string]any) []string {
	titles := []string{}
	for _, raw := range item["todos"].([]any) {
		titles = append(titles, raw.(map[string]any)["title"].(string))
	}
	return titles
}

func pendingTodoItem(t *testing.T, body map[string]any, code string) map[string]any {
	t.Helper()
	for _, raw := range body["items"].([]any) {
		if item := raw.(map[string]any); item["code"] == code {
			return item
		}
	}
	t.Fatalf("no issue %s in %v", code, body)
	return nil
}

func TestPendingTodosListsOnlyTheNotDoneTodosOfTheInProgressIssuesGroupedUnderTheirIssue(t *testing.T) {
	f := newPendingTodosFixture(t)

	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book", nil)

	if got["total"] != float64(3) || got["total_pending_todos"] != float64(4) {
		t.Fatalf("total %v, total_pending_todos %v: %v", got["total"], got["total_pending_todos"], got)
	}
	if len(pendingTodoCodes(got)) != 3 {
		t.Fatalf("issues %v", pendingTodoCodes(got))
	}
	a := pendingTodoItem(t, got, "A")
	if strings.Join(pendingTodoTitles(a), "|") != "a-todo|a-blocked" || a["done"] != float64(1) || a["total"] != float64(3) {
		t.Fatalf("A %v", a)
	}
	b := pendingTodoItem(t, got, "B")
	if strings.Join(pendingTodoTitles(b), "|") != "b-blocked|b-todo" || b["done"] != float64(1) || b["total"] != float64(3) {
		t.Fatalf("B %v", b)
	}
}

func TestPendingTodosCarriesTheIssueCompactAndEachTodoWithItsState(t *testing.T) {
	f := newPendingTodosFixture(t)

	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book", nil)

	a := pendingTodoItem(t, got, "A")
	for _, field := range []string{"id", "code", "title", "priority", "tags", "step", "todos", "done", "total"} {
		if _, has := a[field]; !has {
			t.Errorf("the issue has no %s: %v", field, a)
		}
	}
	if _, has := a["description"]; has {
		t.Errorf("the issue carries a description: %v", a)
	}
	if a["id"] != f.issues["A"] {
		t.Errorf("id %v", a["id"])
	}
	todos := a["todos"].([]any)
	todo, blocked := todos[0].(map[string]any), todos[1].(map[string]any)
	if todo["state"] != "To do" || blocked["state"] != "Blocked" || blocked["blocked_reason"] != "waits on the owner" {
		t.Errorf("todos %v", todos)
	}
	for _, field := range []string{"id", "code", "title", "ordinality", "state"} {
		if _, has := todo[field]; !has {
			t.Errorf("the todo has no %s: %v", field, todo)
		}
	}
	if todo["ordinality"].(float64) >= blocked["ordinality"].(float64) {
		t.Errorf("not in order: %v", todos)
	}
}

func TestPendingTodosNeverListsAnIssueThatIsNotInProgress(t *testing.T) {
	f := newPendingTodosFixture(t)

	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book", nil)

	for _, code := range pendingTodoCodes(got) {
		if code == "C" || code == "F" || code == "E" {
			t.Errorf("issue %s is listed: %v", code, got)
		}
	}
}

func TestPendingTodosListsAnInProgressIssueWithNoPendingTodoWithAnEmptyList(t *testing.T) {
	f := newPendingTodosFixture(t)

	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book", nil)

	d := pendingTodoItem(t, got, "D")
	if len(d["todos"].([]any)) != 0 || d["done"] != float64(1) || d["total"] != float64(1) {
		t.Fatalf("D %v", d)
	}
}

func TestPendingTodosMatchesTheBacklogByIdOrName(t *testing.T) {
	f := newPendingTodosFixture(t)

	byName := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=Elsewhere", nil)

	if strings.Join(pendingTodoCodes(byName), "|") != "E" || byName["total_pending_todos"] != float64(1) {
		t.Fatalf("got %v", byName)
	}
}

func TestPendingTodosRefusesABacklogThePersonCannotSeeExactlyAsAnUnknownOne(t *testing.T) {
	f := newPendingTodosFixture(t)

	hiddenStatus, hidden := f.h.call("dev", "GET", "/pending-todos?backlog=elsewhere", nil)
	unknownStatus, unknown := f.h.call("dev", "GET", "/pending-todos?backlog=nothing-here", nil)

	if hiddenStatus != http.StatusBadRequest || unknownStatus != http.StatusBadRequest {
		t.Fatalf("hidden answered %d %v, unknown answered %d %v", hiddenStatus, hidden, unknownStatus, unknown)
	}
	hiddenMessage, _ := hidden["error"].(string)
	unknownMessage, _ := unknown["error"].(string)
	if !strings.Contains(unknownMessage, "unknown backlog") || strings.ReplaceAll(hiddenMessage, "elsewhere", "nothing-here") != unknownMessage {
		t.Errorf("the hidden backlog is told apart from an unknown one: %q and %q", hiddenMessage, unknownMessage)
	}
	if status, _ := f.h.call("nobody", "GET", "/pending-todos?backlog=book", nil); status != http.StatusBadRequest {
		t.Errorf("a person with no access got %d, want 400", status)
	}
	if status, body := f.h.call("dev", "GET", "/pending-todos?backlog=book", nil); status != http.StatusOK || body["total"] != float64(3) {
		t.Errorf("a person who can see the backlog got %d %v", status, body)
	}
}

func TestPendingTodosRefusesAMissingOrBlankBacklogWith400(t *testing.T) {
	f := newPendingTodosFixture(t)

	for _, path := range []string{"/pending-todos", "/pending-todos?backlog=", "/pending-todos?backlog=%20%20"} {
		status, body := f.h.call("owner", "GET", path, nil)
		if status != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400", path, status)
		}
		if message, _ := body["error"].(string); !strings.Contains(message, "backlog") {
			t.Errorf("%s gave no message naming backlog: %v", path, body)
		}
	}
}

func TestPendingTodosPagesTheIssuesAndKeepsTheTotalsOverAllOfThem(t *testing.T) {
	f := newPendingTodosFixture(t)
	all := pendingTodoCodes(f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book", nil))

	first := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book&limit=2", nil)
	second := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book&limit=2&offset=2", nil)

	if len(pendingTodoCodes(first)) != 2 || len(pendingTodoCodes(second)) != 1 {
		t.Fatalf("pages %v and %v", pendingTodoCodes(first), pendingTodoCodes(second))
	}
	if strings.Join(append(pendingTodoCodes(first), pendingTodoCodes(second)...), "|") != strings.Join(all, "|") {
		t.Errorf("pages do not follow the full list %v", all)
	}
	for _, page := range []map[string]any{first, second} {
		if page["total"] != float64(3) || page["total_pending_todos"] != float64(4) {
			t.Errorf("totals changed with the page: %v", page)
		}
	}
}

func TestPendingTodosRefusesANegativeOffsetAndClampsAnOversizedLimit(t *testing.T) {
	f := newPendingTodosFixture(t)

	if status, _ := f.h.call("owner", "GET", "/pending-todos?backlog=book&offset=-1", nil); status != http.StatusBadRequest {
		t.Errorf("offset -1 answered %d, want 400", status)
	}
	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book&limit=500", nil)
	if len(pendingTodoCodes(got)) != 3 {
		t.Errorf("an oversized limit lists %v", pendingTodoCodes(got))
	}
}

func TestPendingTodosIsAnMCPToolNamedByItsAction(t *testing.T) {
	for _, tool := range routes.ToolShape() {
		if tool.Action != "taskmanager.task.todo.pending" {
			continue
		}
		if tool.Name != "taskmanager_task_todo_pending" {
			t.Fatalf("the tool is named %q", tool.Name)
		}
		for _, field := range []string{"backlog", "limit", "offset"} {
			if !strings.Contains(string(tool.InputSchema), `"`+field+`"`) {
				t.Errorf("the input schema has no %s: %s", field, tool.InputSchema)
			}
		}
		if strings.Contains(string(tool.InputSchema), "requester_id") {
			t.Errorf("the caller is asked for in the input: %s", tool.InputSchema)
		}
		return
	}
	t.Fatal("no tool for taskmanager.task.todo.pending")
}

func (f *pendingTodosFixture) issueMovedTo(code, backlog, step string) string {
	f.h.t.Helper()
	made := f.h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "issue " + code, "code": code, "backlog": backlog})
	id := made["id"].(string)
	f.issues[code] = id
	moved := f.h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": backlog, "step": step})
	return moved["status"].(string)
}

func TestPendingTodosListsAnIssueAtTheInProgressStepWhoseStatusIsStillPending(t *testing.T) {
	f := newPendingTodosFixture(t)
	if status := f.issueMovedTo("G", "book", "in-progress"); status != "pending" {
		t.Fatalf("moving to the step set the status %s", status)
	}
	f.todos("G", "g-todo")

	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book", nil)

	g := pendingTodoItem(t, got, "G")
	if strings.Join(pendingTodoTitles(g), "|") != "g-todo" || got["total"] != float64(4) {
		t.Fatalf("G %v, total %v", g, got["total"])
	}
}

func TestPendingTodosNeverListsAnIssueAtAnotherStepWhoseStatusIsInProgress(t *testing.T) {
	f := newPendingTodosFixture(t)
	f.issueMovedTo("H", "book", "open")
	f.h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+f.issues["H"], map[string]any{"status": "in_progress"})

	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=book", nil)

	for _, code := range pendingTodoCodes(got) {
		if code == "H" {
			t.Errorf("issue %s is listed: %v", code, got)
		}
	}
}

func TestPendingTodosFindsTheInProgressStepWhateverItsIdIs(t *testing.T) {
	f := newPendingTodosFixture(t)
	f.h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Board", "steps": []string{"Open", "Doing", "Done"}})
	f.h.expect(http.StatusOK, "owner", "PUT", "/backlogs/board/steps/doing", map[string]string{"name": "In Progress"})
	f.issueMovedTo("J", "board", "doing")
	f.issueMovedTo("K", "board", "open")
	f.h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+f.issues["K"], map[string]any{"status": "in_progress"})

	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=board", nil)

	if strings.Join(pendingTodoCodes(got), "|") != "J" {
		t.Fatalf("the step id doing listed %v", pendingTodoCodes(got))
	}
}

func TestPendingTodosListsNothingForABacklogWithNoInProgressStep(t *testing.T) {
	f := newPendingTodosFixture(t)
	f.h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Plain", "steps": []string{"Open", "Done"}})
	f.issueMovedTo("L", "plain", "open")
	f.h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+f.issues["L"], map[string]any{"status": "in_progress"})

	got := f.h.expect(http.StatusOK, "owner", "GET", "/pending-todos?backlog=plain", nil)

	if got["total"] != float64(0) || len(pendingTodoCodes(got)) != 0 {
		t.Fatalf("got %v", got)
	}
}
