package routes_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func (f *pendingTodosFixture) waitingOn(issueCode, title, waitsOn string) {
	f.h.t.Helper()
	made := f.h.expect(http.StatusCreated, "owner", "POST", "/todos", map[string]any{
		"title": title, "instructions": "do " + title, "parent_task_id": f.issues[issueCode],
		"depends_on": []string{f.issues[waitsOn]},
	})
	f.issues[title] = made["id"].(string)
}

func newReadyBoardFixture(t *testing.T) *pendingTodosFixture {
	t.Helper()
	f := newPendingTodosFixture(t)
	f.waitingOn("A", "a-after", "a-todo")
	return f
}

func readyBoardTitles(t *testing.T, body map[string]any) string {
	t.Helper()
	titles := []string{}
	for _, raw := range body["items"].([]any) {
		titles = append(titles, raw.(map[string]any)["title"].(string))
	}
	return strings.Join(titles, ",")
}

func TestTheReadyBoardListsTheStartableTodosOfTheIssuesAtTheInProgressStep(t *testing.T) {
	f := newReadyBoardFixture(t)

	got := f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board", nil)

	if want := "a-todo,b-todo,e-todo"; readyBoardTitles(t, got) != want {
		t.Fatalf("got %q, want %q (%v)", readyBoardTitles(t, got), want, got)
	}
	if got["total"] != float64(3) {
		t.Fatalf("total %v", got["total"])
	}
}

func TestTheReadyBoardLeavesOutDoneHandBlockedAndPredecessorBlockedTodos(t *testing.T) {
	f := newReadyBoardFixture(t)

	titles := readyBoardTitles(t, f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board", nil))

	for _, absent := range []string{"a-done", "a-blocked", "a-after", "b-done", "b-blocked", "d-done", "c-todo"} {
		if strings.Contains(titles, absent) {
			t.Errorf("%s is listed in %q", absent, titles)
		}
	}
}

func TestTheReadyBoardListsATodoOnceItsPredecessorIsDone(t *testing.T) {
	f := newReadyBoardFixture(t)
	f.setState("a-todo", "Done")

	titles := readyBoardTitles(t, f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board", nil))

	if want := "a-after,b-todo,e-todo"; titles != want {
		t.Fatalf("got %q, want %q", titles, want)
	}
}

func TestAReadyBoardItemCarriesTheTodoAndItsIssue(t *testing.T) {
	f := newReadyBoardFixture(t)

	got := f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board", nil)
	item := got["items"].([]any)[0].(map[string]any)

	for _, field := range []string{"id", "code", "title", "instructions", "can_run_parallel", "state", "ordinality", "issue_id", "issue_code", "issue_files"} {
		if _, present := item[field]; !present {
			t.Fatalf("no %s in %v", field, item)
		}
	}
	if item["issue_code"] != "A" || item["issue_id"] != f.issues["A"] || item["instructions"] != "do a-todo" {
		t.Fatalf("got %v", item)
	}
}

func TestTheReadyBoardLimitCutsTheItemsButTotalCountsEveryMatch(t *testing.T) {
	f := newReadyBoardFixture(t)

	got := f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board?limit=2", nil)

	if want := "a-todo,b-todo"; readyBoardTitles(t, got) != want || got["total"] != float64(3) {
		t.Fatalf("got %q total %v", readyBoardTitles(t, got), got["total"])
	}
}

func TestTheReadyBoardRefusesALimitAbove100AndBelow1(t *testing.T) {
	f := newReadyBoardFixture(t)

	f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board?limit=100", nil)
	f.h.expect(http.StatusBadRequest, "owner", "GET", "/todo-ready-board?limit=101", nil)
	f.h.expect(http.StatusBadRequest, "owner", "GET", "/todo-ready-board?limit=-1", nil)
}

func TestTheReadyBoardNarrowsToOneBacklog(t *testing.T) {
	f := newReadyBoardFixture(t)

	got := f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board?backlog=elsewhere", nil)

	if readyBoardTitles(t, got) != "e-todo" || got["total"] != float64(1) {
		t.Fatalf("got %v", got)
	}
}

func TestTheReadyBoardRefusesABacklogTheCallerCannotSee(t *testing.T) {
	f := newReadyBoardFixture(t)

	status, _ := f.h.call("dev", "GET", "/todo-ready-board?backlog=elsewhere", nil)

	if status != http.StatusBadRequest {
		t.Fatalf("status %d", status)
	}
}

func TestTheReadyBoardShowsACallerOnlyTheBacklogsTheirGroupsSee(t *testing.T) {
	f := newReadyBoardFixture(t)

	dev := f.h.expect(http.StatusOK, "dev", "GET", "/todo-ready-board", nil)
	nobody := f.h.expect(http.StatusOK, "nobody", "GET", "/todo-ready-board", nil)

	if readyBoardTitles(t, dev) != "a-todo,b-todo" || dev["total"] != float64(2) {
		t.Fatalf("dev got %v", dev)
	}
	if readyBoardTitles(t, nobody) != "" || nobody["total"] != float64(0) {
		t.Fatalf("nobody got %v", nobody)
	}
}

func TestTheReadyBoardNarrowsByTag(t *testing.T) {
	f := newReadyBoardFixture(t)
	f.h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+f.issues["B"], map[string]any{"tags": []string{"Ready"}})

	got := f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board?tag=Ready", nil)

	if readyBoardTitles(t, got) != "b-todo" || got["total"] != float64(1) {
		t.Fatalf("got %v", got)
	}
}

func TestTheReadyBoardAnswersAnEmptyListWhenNothingIsInProgress(t *testing.T) {
	f := newReadyBoardFixture(t)
	for _, title := range []string{"a-todo", "b-todo", "e-todo"} {
		f.setState(title, "Done")
	}
	f.setState("a-after", "Done")

	got := f.h.expect(http.StatusOK, "owner", "GET", "/todo-ready-board", nil)

	if len(got["items"].([]any)) != 0 || got["total"] != float64(0) {
		t.Fatalf("got %v", got)
	}
}

func TestTheReadyBoardIsAnMCPToolNamedByItsAction(t *testing.T) {
	for _, tool := range routes.ToolShape() {
		if tool.Action != "taskmanager.task_todo.ready_board" {
			continue
		}
		if tool.Name != "taskmanager_task_todo_ready_board" {
			t.Fatalf("the tool is named %q", tool.Name)
		}
		for _, field := range []string{"backlog", "tag", "limit"} {
			if !strings.Contains(string(tool.InputSchema), `"`+field+`"`) {
				t.Errorf("the input schema has no %s: %s", field, tool.InputSchema)
			}
		}
		if strings.Contains(string(tool.InputSchema), "requester_id") {
			t.Errorf("the caller is asked for in the input: %s", tool.InputSchema)
		}
		return
	}
	t.Fatal("no tool for taskmanager.task_todo.ready_board")
}
