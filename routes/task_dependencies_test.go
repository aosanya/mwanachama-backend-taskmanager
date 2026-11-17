package routes_test

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
)

type dependencyHub struct {
	h                                     *accessHub
	subject, open, done, waiterA, waiterB string
	hiddenIssue, hiddenWaiter             string
}

func newDependencyHub(t *testing.T) *dependencyHub {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"In coming", "To do", "Done"}})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Hidden", "steps": []string{"In coming", "To do", "Done"}})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	file := func(code, backlog string) string {
		made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "Title " + code, "code": code})
		id := made["id"].(string)
		h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": backlog, "step": "to-do"})
		return id
	}
	f := &dependencyHub{h: h}
	f.subject = file("SW1", "software")
	f.open = file("SW2", "software")
	f.done = file("SW3", "software")
	f.waiterA = file("SW4", "software")
	f.waiterB = file("SW5", "software")
	f.hiddenIssue = file("HD1", "hidden")
	f.hiddenWaiter = file("HD2", "hidden")
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+f.done+"/move", map[string]any{"state": "Done"})
	for _, edge := range [][2]string{
		{f.subject, f.open}, {f.subject, f.done}, {f.waiterA, f.subject}, {f.waiterB, f.subject},
	} {
		h.expect(http.StatusCreated, "owner", "POST", "/relationships", map[string]any{"label": "depends_on", "from_id": edge[0], "to_id": edge[1]})
	}
	return f
}

func dependencyItems(t *testing.T, body map[string]any, side string) map[string]map[string]any {
	t.Helper()
	items := map[string]map[string]any{}
	list, _ := body[side].([]any)
	for _, raw := range list {
		entry := raw.(map[string]any)
		items[entry["code"].(string)] = entry
	}
	return items
}

func wantOpenFlags(t *testing.T, items map[string]map[string]any, want map[string]bool) {
	t.Helper()
	var got []string
	for code, entry := range items {
		got = append(got, code)
		open, known := want[code]
		if !known || entry["open"] != open {
			t.Errorf("%s open = %v, want %v (known %v): %v", code, entry["open"], open, known, entry)
		}
		for _, field := range []string{"id", "code", "title", "step", "state", "open"} {
			if _, has := entry[field]; !has {
				t.Errorf("%s has no %s: %v", code, field, entry)
			}
		}
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Errorf("got %s, want %d items", strings.Join(got, ","), len(want))
	}
}

func TestTaskDependenciesNamesWhatAnIssueWaitsOnAndWhatWaitsOnItWithOpenFlags(t *testing.T) {
	f := newDependencyHub(t)

	body := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/dependencies", nil)

	wantOpenFlags(t, dependencyItems(t, body, "depends_on"), map[string]bool{"SW2": true, "SW3": false})
	wantOpenFlags(t, dependencyItems(t, body, "needed_by"), map[string]bool{"SW4": true, "SW5": true})
	if body["blocks_start"] != true {
		t.Errorf("blocks_start = %v, want true while SW2 is open", body["blocks_start"])
	}
	done := dependencyItems(t, body, "depends_on")["SW3"]
	if done["state"] != "Done" || done["title"] != "Title SW3" {
		t.Errorf("done dependency = %v", done)
	}
}

func TestTaskDependenciesBlocksStartIsFalseOnceTheOpenDependencyIsFinished(t *testing.T) {
	f := newDependencyHub(t)
	f.h.expect(http.StatusOK, "owner", "POST", "/tasks/"+f.open+"/move", map[string]any{"state": "Done"})

	body := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/dependencies", nil)

	wantOpenFlags(t, dependencyItems(t, body, "depends_on"), map[string]bool{"SW2": false, "SW3": false})
	if body["blocks_start"] != false {
		t.Errorf("blocks_start = %v, want false", body["blocks_start"])
	}
}

func TestTaskDependenciesAnIssueThatWaitsOnNothingHasEmptyListsAndStartsFree(t *testing.T) {
	f := newDependencyHub(t)

	body := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.open+"/dependencies", nil)

	if list, ok := body["depends_on"].([]any); !ok || len(list) != 0 {
		t.Errorf("depends_on = %v, want an empty list", body["depends_on"])
	}
	if body["blocks_start"] != false {
		t.Errorf("blocks_start = %v, want false", body["blocks_start"])
	}
}

func TestTaskDependenciesOnlyCountsWhatTheCallerCannotSee(t *testing.T) {
	f := newDependencyHub(t)
	f.h.expect(http.StatusCreated, "owner", "POST", "/relationships", map[string]any{"label": "depends_on", "from_id": f.subject, "to_id": f.hiddenIssue})
	f.h.expect(http.StatusCreated, "owner", "POST", "/relationships", map[string]any{"label": "depends_on", "from_id": f.hiddenWaiter, "to_id": f.subject})

	seen := f.h.expect(http.StatusOK, "dev", "GET", "/tasks/"+f.subject+"/dependencies", nil)

	wantOpenFlags(t, dependencyItems(t, seen, "depends_on"), map[string]bool{"SW2": true, "SW3": false})
	wantOpenFlags(t, dependencyItems(t, seen, "needed_by"), map[string]bool{"SW4": true, "SW5": true})
	if seen["hidden_depends_on"] != float64(1) || seen["hidden_needed_by"] != float64(1) {
		t.Errorf("hidden counts = %v and %v, want 1 and 1", seen["hidden_depends_on"], seen["hidden_needed_by"])
	}
	if strings.Contains(fmt.Sprint(seen), "HD") {
		t.Errorf("the answer names a hidden issue")
	}
	owner := f.h.expect(http.StatusOK, "owner", "GET", "/tasks/"+f.subject+"/dependencies", nil)
	if owner["hidden_depends_on"] != float64(0) || owner["hidden_needed_by"] != float64(0) {
		t.Errorf("owner hidden counts = %v and %v, want 0 and 0", owner["hidden_depends_on"], owner["hidden_needed_by"])
	}
	wantOpenFlags(t, dependencyItems(t, owner, "depends_on"), map[string]bool{"SW2": true, "SW3": false, "HD1": true})
}

func TestTaskDependenciesOfAnIssueTheCallerCannotSeeIsNotFound(t *testing.T) {
	f := newDependencyHub(t)

	f.h.expect(http.StatusNotFound, "dev", "GET", "/tasks/"+f.hiddenIssue+"/dependencies", nil)
	f.h.expect(http.StatusNotFound, "nobody", "GET", "/tasks/"+f.subject+"/dependencies", nil)
	f.h.expect(http.StatusNotFound, "owner", "GET", "/tasks/no-such-issue/dependencies", nil)
}
