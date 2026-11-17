package routes_test

import (
	"net/http"
	"testing"
)

func (f *moreWritesFixture) readyTodos(t *testing.T) map[string]any {
	t.Helper()
	return f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/todos/ready", nil)
}

func (f *moreWritesFixture) setTodo(t *testing.T, id, status string) {
	t.Helper()
	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+id+"/status", map[string]any{"status": status})
}

func TestTheReadyTodosAreThoseWhosePredecessorsAreAllDone(t *testing.T) {
	f := newMoreWritesFixture(t)
	one := f.todo(t, f.issue, "one")
	two := f.todo(t, f.issue, "two", one)
	f.todo(t, f.issue, "three", one)
	f.todo(t, f.issue, "four", two)
	f.setTodo(t, one, "Done")

	ready := f.readyTodos(t)

	if !sameTitles(todoTitles(ready), "two", "three") {
		t.Fatalf("got %v", ready)
	}
	if ready["done"] != float64(1) || ready["total"] != float64(4) {
		t.Fatalf("done and total %v", ready)
	}

	f.setTodo(t, two, "Done")

	if got := todoTitles(f.readyTodos(t)); !sameTitles(got, "three", "four") {
		t.Fatalf("after two is done got %v", got)
	}
}

func TestAReadyTodoCarriesWhatASubAgentNeedsToStart(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.todo(t, f.issue, "one")

	item := todoNamed(t, f.readyTodos(t), "one")

	for _, field := range []string{"id", "code", "title", "instructions", "ordinality", "state", "can_run_parallel"} {
		if _, present := item[field]; !present {
			t.Fatalf("no %s in %v", field, item)
		}
	}
	if item["state"] != "To do" || item["instructions"] != "do one" {
		t.Fatalf("got %v", item)
	}
}

func TestATodoInProgressIsStillListedAsReady(t *testing.T) {
	f := newMoreWritesFixture(t)
	one := f.todo(t, f.issue, "one")
	f.setTodo(t, one, "In progress")

	item := todoNamed(t, f.readyTodos(t), "one")

	if item["state"] != "In progress" {
		t.Fatalf("got %v", item)
	}
}

func TestATodoBehindABlockedOneIsNotReadyAndTheBlockedOneIsListedWithItsReason(t *testing.T) {
	f := newMoreWritesFixture(t)
	one := f.todo(t, f.issue, "one")
	f.todo(t, f.issue, "two", one)
	f.todo(t, f.issue, "free")
	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+one+"/status", map[string]any{"status": "Blocked", "blocked_reason": "waiting for the key"})

	ready := f.readyTodos(t)

	if !sameTitles(todoTitles(ready), "free") {
		t.Fatalf("got %v", ready)
	}
	blocked, _ := ready["blocked"].([]any)
	if len(blocked) != 1 {
		t.Fatalf("blocked %v", ready["blocked"])
	}
	entry := blocked[0].(map[string]any)
	if entry["title"] != "one" || entry["blocked_reason"] != "waiting for the key" {
		t.Fatalf("got %v", entry)
	}
}

func TestAnIssueWithNothingReadyListsEmptyArrays(t *testing.T) {
	f := newMoreWritesFixture(t)

	ready := f.readyTodos(t)

	items, itemsOK := ready["items"].([]any)
	blocked, blockedOK := ready["blocked"].([]any)
	if !itemsOK || !blockedOK || len(items) != 0 || len(blocked) != 0 || ready["total"] != float64(0) {
		t.Fatalf("got %v", ready)
	}
}

func TestReadyTodosOfAnIssueTheCallerCannotSeeAre404(t *testing.T) {
	f := newMoreWritesFixture(t)

	f.h.expect(http.StatusNotFound, "boss", "GET", "/tasks/nope/todos/ready", nil)
}
