package routes_test

import (
	"net/http"
	"testing"
)

func (f *moreWritesFixture) taskReport(t *testing.T) map[string]any {
	t.Helper()
	return f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue+"/report", nil)
}

func (f *moreWritesFixture) logNote(t *testing.T, text string, links ...string) string {
	t.Helper()
	body := map[string]any{"kind": "worker", "text": text, "links": links}
	return f.h.expect(http.StatusCreated, "boss", "POST", "/tasks/"+f.issue+"/notes", body)["id"].(string)
}

func stringsOf(t *testing.T, report map[string]any, field string) []string {
	t.Helper()
	list, ok := report[field].([]any)
	if !ok {
		t.Fatalf("%s is not a list in %v", field, report)
	}
	var out []string
	for _, raw := range list {
		out = append(out, raw.(string))
	}
	return out
}

func TestTheReportCountsTodosCommitsEntriesAndTheTodoWithNoEntry(t *testing.T) {
	f := newMoreWritesFixture(t)
	one := f.todo(t, f.issue, "one")
	two := f.todo(t, f.issue, "two")
	three := f.todo(t, f.issue, "three")
	for _, id := range []string{one, two, three} {
		f.setTodo(t, id, "Done")
	}
	f.logNote(t, "Taken, caps in force.")
	f.logNote(t, "Todo 1 of 3 done: the failing test.", "commit aaa11111 one.go")
	f.logNote(t, "Todo 2 of 3 done: the code.", "commit bbb22222 two.go", "CI run https://ci.example/1")
	last := f.logNote(t, "Committed again.", "commit aaa11111 one.go")

	report := f.taskReport(t)

	if report["todos_done"] != float64(3) || report["todos_total"] != float64(3) {
		t.Fatalf("todos %v", report)
	}
	if got := stringsOf(t, report, "commits"); !sameTitles(got, "commit aaa11111 one.go", "commit bbb22222 two.go") {
		t.Fatalf("commits %v", got)
	}
	if report["entry_count"] != float64(4) || report["last_entry_id"] != last {
		t.Fatalf("entries %v", report)
	}
	missing, _ := report["missing"].([]any)
	if len(missing) != 1 || missing[0].(map[string]any)["title"] != "three" {
		t.Fatalf("missing %v", report["missing"])
	}
}

func TestTheReportNamesTheStepTheTagsAndTheTimesOfTheLog(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.logNote(t, "Taken.")
	f.logNote(t, "Moved to review.")

	report := f.taskReport(t)

	if report["step"] == "" || report["step"] == nil {
		t.Fatalf("step %v", report)
	}
	if _, ok := report["tags"].([]any); !ok {
		t.Fatalf("tags %v", report)
	}
	first, _ := report["first_entry_at"].(string)
	last, _ := report["last_entry_at"].(string)
	if first == "" || last == "" || last < first {
		t.Fatalf("times %v", report)
	}
	if _, ok := report["minutes_between"].(float64); !ok {
		t.Fatalf("minutes_between %v", report)
	}
}

func TestABlockedTodoIsListedWithItsReasonAndOneWithAnEntryIsNotMissing(t *testing.T) {
	f := newMoreWritesFixture(t)
	one := f.todo(t, f.issue, "one")
	two := f.todo(t, f.issue, "two")
	f.setTodo(t, one, "Done")
	f.h.expect(http.StatusOK, "boss", "PUT", "/todos/"+two+"/status", map[string]any{"status": "Blocked", "blocked_reason": "waiting for the key"})
	f.logNote(t, "Todo 1 of 2 done: written.")

	report := f.taskReport(t)

	if report["todos_done"] != float64(1) || report["todos_total"] != float64(2) {
		t.Fatalf("todos %v", report)
	}
	blocked, _ := report["blocked"].([]any)
	if len(blocked) != 1 || blocked[0].(map[string]any)["blocked_reason"] != "waiting for the key" || blocked[0].(map[string]any)["title"] != "two" {
		t.Fatalf("blocked %v", report["blocked"])
	}
	if missing, _ := report["missing"].([]any); len(missing) != 0 {
		t.Fatalf("missing %v", missing)
	}
}

func TestAnIssueWithNoLogReportsZeroEntriesAndNullTimes(t *testing.T) {
	f := newMoreWritesFixture(t)

	report := f.taskReport(t)

	if report["entry_count"] != float64(0) || report["last_entry_id"] != nil {
		t.Fatalf("entries %v", report)
	}
	for _, field := range []string{"first_entry_at", "last_entry_at", "minutes_between"} {
		value, present := report[field]
		if !present || value != nil {
			t.Fatalf("%s should be null in %v", field, report)
		}
	}
	if got := stringsOf(t, report, "commits"); len(got) != 0 {
		t.Fatalf("commits %v", got)
	}
	if report["todos_done"] != float64(0) || report["todos_total"] != float64(0) {
		t.Fatalf("todos %v", report)
	}
}

func TestTheReportOfAnIssueTheCallerCannotSeeIs404(t *testing.T) {
	f := newMoreWritesFixture(t)

	f.h.expect(http.StatusNotFound, "boss", "GET", "/tasks/nope/report", nil)
	f.h.expect(http.StatusNotFound, "nog", "GET", "/tasks/"+f.issue+"/report", nil)
}
