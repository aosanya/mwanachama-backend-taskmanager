package routes_test

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func (f *dependencyHub) note(owner, issue, kind, text string) {
	f.h.expect(http.StatusCreated, owner, "POST", "/tasks/"+issue+"/notes", map[string]any{"kind": kind, "text": text})
}

func briefTexts(body map[string]any, field string) []string {
	var texts []string
	for _, raw := range body[field].([]any) {
		texts = append(texts, raw.(map[string]any)["text"].(string))
	}
	return texts
}

func TestTaskBriefReturnsTheLastTenEntriesAndEveryPersonNoteBeyondThem(t *testing.T) {
	f := newDependencyHub(t)
	for i := 1; i <= 12; i++ {
		kind := "worker"
		if i == 2 {
			kind = "person"
		}
		f.note("owner", f.subject, kind, fmt.Sprintf("entry %d", i))
	}

	body := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/brief", nil)

	want := []string{"entry 2", "entry 3", "entry 4", "entry 5", "entry 6", "entry 7", "entry 8", "entry 9", "entry 10", "entry 11", "entry 12"}
	got := briefTexts(body, "log")
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("log = %v, want the last ten plus the person note: %v", got, want)
	}
	issue := body["issue"].(map[string]any)
	if issue["code"] != "SW1" || issue["title"] != "Title SW1" {
		t.Errorf("issue = %v", issue)
	}
}

func TestTaskBriefLogLimitCutsTheWorkerEntriesAndKeepsThePersonNotes(t *testing.T) {
	f := newDependencyHub(t)
	for i := 1; i <= 6; i++ {
		kind := "worker"
		if i == 1 {
			kind = "person"
		}
		f.note("owner", f.subject, kind, fmt.Sprintf("entry %d", i))
	}

	body := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/brief?log_limit=2", nil)

	if got := strings.Join(briefTexts(body, "log"), ","); got != "entry 1,entry 5,entry 6" {
		t.Errorf("log = %s", got)
	}
	f.h.expect(http.StatusBadRequest, "owner", "GET", "/tasks/"+f.subject+"/brief?log_limit=0", nil)
	f.h.expect(http.StatusBadRequest, "owner", "GET", "/tasks/"+f.subject+"/brief?log_limit=51", nil)
}

func TestTaskBriefDependenciesCarryTheirLastThreeEntriesAndOpenFlags(t *testing.T) {
	f := newDependencyHub(t)
	for i := 1; i <= 5; i++ {
		f.note("owner", f.open, "worker", fmt.Sprintf("open %d", i))
	}
	f.note("owner", f.done, "worker", "finished")

	body := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/brief", nil)

	dependencies := dependencyItems(t, body, "depends_on")
	open, done := dependencies["SW2"], dependencies["SW3"]
	if open["open"] != true || done["open"] != false {
		t.Errorf("open flags = %v and %v", open["open"], done["open"])
	}
	var entries []string
	for _, raw := range open["last_entries"].([]any) {
		entries = append(entries, raw.(map[string]any)["text"].(string))
	}
	if strings.Join(entries, ",") != "open 3,open 4,open 5" {
		t.Errorf("last entries of the open dependency = %v", entries)
	}
	var finished []string
	for _, raw := range done["last_entries"].([]any) {
		finished = append(finished, raw.(map[string]any)["text"].(string))
	}
	if !slices.Contains(finished, "finished") {
		t.Errorf("last entries of the finished dependency = %v", finished)
	}
	wantOpenFlags2 := dependencyItems(t, body, "needed_by")
	if len(wantOpenFlags2) != 2 || wantOpenFlags2["SW4"]["title"] != "Title SW4" {
		t.Errorf("needed_by = %v", wantOpenFlags2)
	}
}

func TestTaskBriefCountsADependencyTheCallerCannotSeeAndNeverNamesIt(t *testing.T) {
	f := newDependencyHub(t)
	f.h.expect(http.StatusCreated, "owner", "POST", "/relationships", map[string]any{"label": "depends_on", "from_id": f.subject, "to_id": f.hiddenIssue})
	f.note("owner", f.hiddenIssue, "worker", "secret entry")

	seen := f.h.expect(http.StatusOK, "dev", "GET", "/tasks/"+f.subject+"/brief", nil)

	if seen["hidden_depends_on"] != float64(1) || seen["hidden_needed_by"] != float64(0) {
		t.Errorf("hidden counts = %v and %v", seen["hidden_depends_on"], seen["hidden_needed_by"])
	}
	if text := fmt.Sprint(seen); strings.Contains(text, "HD") || strings.Contains(text, "secret entry") {
		t.Errorf("the brief names a hidden issue: %s", text)
	}
	owner := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/brief", nil)
	if owner["hidden_depends_on"] != float64(0) {
		t.Errorf("owner hidden count = %v", owner["hidden_depends_on"])
	}
}

func TestTaskBriefHoldsTheCapsAndTheTodosWithTheirCounts(t *testing.T) {
	f := newDependencyHub(t)
	f.h.expect(http.StatusOK, "owner", "PUT", "/tasks/"+f.subject+"/caps", map[string]any{"runtime_seconds": 1200})
	first := f.h.expect(http.StatusCreated, "owner", "POST", "/todos", map[string]any{"title": "first", "instructions": "do first", "parent_task_id": f.subject})["id"].(string)
	f.h.expect(http.StatusCreated, "owner", "POST", "/todos", map[string]any{"title": "second", "instructions": "do second", "parent_task_id": f.subject})
	f.h.expect(http.StatusOK, "owner", "PUT", "/todos/"+first+"/status", map[string]any{"status": "Done"})

	body := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/brief", nil)
	caps := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/caps", nil)

	if fmt.Sprint(body["caps"]) != fmt.Sprint(caps) {
		t.Errorf("caps = %v, want %v", body["caps"], caps)
	}
	todos := body["todos"].(map[string]any)
	if todos["done"] != float64(1) || todos["total"] != float64(2) || len(todos["items"].([]any)) != 2 {
		t.Errorf("todos = %v", todos)
	}
}

func TestTaskBriefOfAnIssueTheCallerCannotSeeIsNotFound(t *testing.T) {
	f := newDependencyHub(t)

	f.h.expect(http.StatusNotFound, "dev", "GET", "/tasks/"+f.hiddenIssue+"/brief", nil)
	f.h.expect(http.StatusNotFound, "nobody", "GET", "/tasks/"+f.subject+"/brief", nil)
	f.h.expect(http.StatusNotFound, "owner", "GET", "/tasks/no-such-issue/brief", nil)
}
