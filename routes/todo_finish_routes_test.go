package routes_test

import (
	"net/http"
	"reflect"
	"testing"
)

func TestFinishingATodoSetsItDoneAndAddsOneWorkerLogEntryWithTheTextAndLinks(t *testing.T) {
	f := newMoreWritesFixture(t)
	first := f.todo(t, f.issue, "first")
	f.todo(t, f.issue, "second")
	reply := f.h.expect(http.StatusOK, "boss", "POST", "/todos/"+first+"/finish", map[string]any{"text": "  wrote the first part  ", "links": []string{"https://example.test/c/1", " ", "https://example.test/c/2"}})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/todos/"+first, nil); got["status"] != "completed" {
		t.Fatalf("todo %v", got)
	}
	log := f.h.notesAs("boss", "/tasks/"+f.issue+"/notes")
	if len(log) != 1 || log[0]["kind"] != "worker" || log[0]["text"] != "wrote the first part" {
		t.Fatalf("log %v", log)
	}
	if links := log[0]["links"]; !reflect.DeepEqual(links, []any{"https://example.test/c/1", "https://example.test/c/2"}) {
		t.Fatalf("links %v", links)
	}
	if reply["state"] != "Done" {
		t.Fatalf("reply %v", reply)
	}
}
func TestTheReplyOfFinishingATodoHoldsOnlyItsCodeAndState(t *testing.T) {
	f := newMoreWritesFixture(t)
	first := f.todo(t, f.issue, "first")
	f.todo(t, f.issue, "second")
	code := f.h.expect(http.StatusOK, "boss", "GET", "/todos/"+first, nil)["code"]
	reply := f.h.expect(http.StatusOK, "boss", "POST", "/todos/"+first+"/finish", map[string]any{"text": "done"})
	if !reflect.DeepEqual(reply, map[string]any{"code": code, "state": "Done"}) {
		t.Fatalf("reply %v", reply)
	}
}
func TestFinishingATodoWithBlankTextIsRefusedAndNothingChanges(t *testing.T) {
	f := newMoreWritesFixture(t)
	first := f.todo(t, f.issue, "first")
	f.h.expect(http.StatusBadRequest, "boss", "POST", "/todos/"+first+"/finish", map[string]any{"text": "   "})
	f.h.expect(http.StatusBadRequest, "boss", "POST", "/todos/"+first+"/finish", map[string]any{"links": []string{"https://example.test"}})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/todos/"+first, nil); got["status"] != "pending" {
		t.Fatalf("todo %v", got)
	}
	if log := f.h.notesAs("boss", "/tasks/"+f.issue+"/notes"); len(log) != 0 {
		t.Fatalf("log %v", log)
	}
}
func TestFinishingAnUnknownTodoIsNotFound(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.h.expect(http.StatusNotFound, "boss", "POST", "/todos/nope/finish", map[string]any{"text": "done"})
	if log := f.h.notesAs("boss", "/tasks/"+f.issue+"/notes"); len(log) != 0 {
		t.Fatalf("log %v", log)
	}
}
func TestAReaderCannotFinishATodoAndNothingIsWritten(t *testing.T) {
	f := newMoreWritesFixture(t)
	first := f.todo(t, f.issue, "first")
	f.h.expect(http.StatusForbidden, "ann", "POST", "/todos/"+first+"/finish", map[string]any{"text": "done"})
	f.h.expect(http.StatusNotFound, "nog", "POST", "/todos/"+first+"/finish", map[string]any{"text": "done"})
	if got := f.h.expect(http.StatusOK, "boss", "GET", "/todos/"+first, nil); got["status"] != "pending" {
		t.Fatalf("todo %v", got)
	}
	if log := f.h.notesAs("boss", "/tasks/"+f.issue+"/notes"); len(log) != 0 {
		t.Fatalf("log %v", log)
	}
}
