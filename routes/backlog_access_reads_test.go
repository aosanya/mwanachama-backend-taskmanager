package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"
)

type readFixture struct {
	h        *accessHub
	hidden   string
	visible  string
	visible2 string
}

func (f *readFixture) raw(caller, method, path string) (int, []byte) {
	f.h.t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), callerKey{}, caller))
	rec := httptest.NewRecorder()
	f.h.mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func (f *readFixture) titles(caller, path string) []string {
	f.h.t.Helper()
	code, body := f.raw(caller, "GET", path)
	if code != http.StatusOK {
		f.h.t.Fatalf("GET %s as %q = %d %s", path, caller, code, body)
	}
	var tasks []map[string]any
	if err := json.Unmarshal(body, &tasks); err != nil {
		f.h.t.Fatalf("GET %s body %s: %v", path, body, err)
	}
	out := []string{}
	for _, task := range tasks {
		out = append(out, task["title"].(string))
	}
	sort.Strings(out)
	return out
}

func (f *readFixture) status(caller, path string) int {
	f.h.t.Helper()
	code, _ := f.raw(caller, "GET", path)
	return code
}

func newReadFixture(t *testing.T) *readFixture {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"Open", "Shipped"}})
	past := time.Now().UTC().Add(-72 * time.Hour).Format(time.RFC3339)
	make := func(title string, intoSoftware bool) string {
		made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": title, "due_at": past, "workflow_run_id": "run1"})
		id := made["id"].(string)
		if intoSoftware {
			h.expect(http.StatusOK, "owner", "POST", "/tasks/"+id+"/move-to-step", map[string]any{"backlog": "software", "step": "open"})
		}
		return id
	}
	f := &readFixture{h: h}
	f.hidden = make("hidden", false)
	f.visible = make("visible", true)
	f.visible2 = make("visible too", true)
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	return f
}

func TestReadingOneIssueHidesAnIssueOfABacklogThePersonCannotSee(t *testing.T) {
	f := newReadFixture(t)
	cases := []struct {
		caller, id string
		want       int
	}{
		{"dev", f.hidden, http.StatusNotFound},
		{"nobody", f.visible, http.StatusNotFound},
		{"dev", f.visible, http.StatusOK},
		{"owner", f.hidden, http.StatusOK},
		{"", f.hidden, http.StatusOK},
	}
	for _, c := range cases {
		if got := f.status(c.caller, "/tasks/"+c.id); got != c.want {
			t.Errorf("GET /tasks/%s as %q = %d, want %d", c.id, c.caller, got, c.want)
		}
	}
}

func TestAnIssueHiddenFromAPersonHasNoAssignmentsToRead(t *testing.T) {
	f := newReadFixture(t)
	if got := f.status("dev", "/tasks/"+f.hidden+"/assignments"); got != http.StatusNotFound {
		t.Errorf("assignments of a hidden issue as dev = %d", got)
	}
	if got := f.status("dev", "/tasks/"+f.visible+"/assignments"); got != http.StatusOK {
		t.Errorf("assignments of a visible issue as dev = %d", got)
	}
	if got := f.status("owner", "/tasks/"+f.hidden+"/assignments"); got != http.StatusOK {
		t.Errorf("assignments as owner = %d", got)
	}
}

func TestOverdueIssuesLeaveOutTheOnesInHiddenBacklogs(t *testing.T) {
	f := newReadFixture(t)
	if got := f.titles("dev", "/overdue-tasks"); len(got) != 2 || got[0] != "visible" || got[1] != "visible too" {
		t.Errorf("dev sees overdue %v", got)
	}
	if got := f.titles("owner", "/overdue-tasks"); len(got) != 3 {
		t.Errorf("owner sees overdue %v", got)
	}
	if got := f.titles("", "/overdue-tasks"); len(got) != 3 {
		t.Errorf("no person sees overdue %v", got)
	}
}

func TestAWorkflowRunsIssuesLeaveOutTheOnesInHiddenBacklogs(t *testing.T) {
	f := newReadFixture(t)
	if got := f.titles("dev", "/workflow-runs/run1/tasks"); len(got) != 2 {
		t.Errorf("dev sees run issues %v", got)
	}
	if got := f.titles("owner", "/workflow-runs/run1/tasks"); len(got) != 3 {
		t.Errorf("owner sees run issues %v", got)
	}
}

func TestAProjectsIssuesAndNamedIssueLeaveOutTheOnesInHiddenBacklogs(t *testing.T) {
	f := newReadFixture(t)
	h := f.h
	project := h.expect(http.StatusCreated, "owner", "POST", "/projects", map[string]any{"name": "proj"})
	projectID := project["id"].(string)
	for _, id := range []string{f.hidden, f.visible} {
		h.expect(http.StatusNoContent, "owner", "POST", "/projects/"+projectID+"/tasks/"+id, nil)
	}
	if got := f.titles("dev", "/projects/"+projectID+"/tasks"); len(got) != 1 || got[0] != "visible" {
		t.Errorf("dev sees project issues %v", got)
	}
	if got := f.titles("owner", "/projects/"+projectID+"/tasks"); len(got) != 2 {
		t.Errorf("owner sees project issues %v", got)
	}
	named := h.expect(http.StatusCreated, "owner", "POST", "/projects/proj/tasks", map[string]any{"title": "named"})
	name, _ := named["task_name"].(string)
	if name == "" {
		t.Fatalf("the project gave the issue no name: %v", named)
	}
	if got := f.status("dev", "/projects/proj/tasks/by-name/"+name); got != http.StatusNotFound {
		t.Errorf("dev reads a hidden issue by name = %d", got)
	}
	if got := f.status("owner", "/projects/proj/tasks/by-name/"+name); got != http.StatusOK {
		t.Errorf("owner reads it by name = %d", got)
	}
}

func TestACasesIssuesLeaveOutTheOnesInHiddenBacklogs(t *testing.T) {
	f := newReadFixture(t)
	h := f.h
	opened := h.expect(http.StatusCreated, "owner", "POST", "/cases", map[string]any{"title": "a case"})
	caseID := opened["id"].(string)
	for _, id := range []string{f.hidden, f.visible} {
		h.expect(http.StatusNoContent, "owner", "PUT", "/cases/"+caseID+"/tasks/"+id, nil)
	}
	if got := f.titles("dev", "/cases/"+caseID+"/tasks"); len(got) != 1 || got[0] != "visible" {
		t.Errorf("dev sees case issues %v", got)
	}
	if got := f.titles("owner", "/cases/"+caseID+"/tasks"); len(got) != 2 {
		t.Errorf("owner sees case issues %v", got)
	}
}

func (f *readFixture) edges(caller, vertex, direction string) int {
	f.h.t.Helper()
	code, body := f.raw(caller, "GET", "/relationships/traverse?label=depends_on&vertex_id="+vertex+"&direction="+direction)
	if code != http.StatusOK {
		f.h.t.Fatalf("traverse %s %s as %q = %d %s", vertex, direction, caller, code, body)
	}
	var list []map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		f.h.t.Fatalf("traverse body %s: %v", body, err)
	}
	return len(list)
}

func TestWhatAnIssueWaitsOnLeavesOutIssuesTheCallerCannotSee(t *testing.T) {
	f := newReadFixture(t)
	h := f.h
	link := func(from, to string) {
		h.expect(http.StatusCreated, "owner", "POST", "/relationships", map[string]any{"label": "depends_on", "from_id": from, "to_id": to})
	}
	link(f.visible, f.hidden)
	link(f.hidden, f.visible)
	link(f.visible, f.visible2)

	if got := f.edges("dev", f.visible, "outbound"); got != 1 {
		t.Errorf("dev: what visible waits on = %d edges, want 1 (the hidden one left out)", got)
	}
	if got := f.edges("dev", f.visible, "inbound"); got != 0 {
		t.Errorf("dev: what waits on visible = %d edges, want 0 (the hidden one left out)", got)
	}
	if got := f.edges("dev", f.hidden, "outbound"); got != 0 {
		t.Errorf("dev: what the hidden issue waits on = %d edges, want 0", got)
	}
	if got := f.edges("dev", f.hidden, "inbound"); got != 0 {
		t.Errorf("dev: what waits on the hidden issue = %d edges, want 0", got)
	}
	for _, person := range []string{"owner", ""} {
		if got := f.edges(person, f.visible, "outbound"); got != 2 {
			t.Errorf("%q: what visible waits on = %d edges, want 2", person, got)
		}
		if got := f.edges(person, f.hidden, "outbound"); got != 1 {
			t.Errorf("%q: what hidden waits on = %d edges, want 1", person, got)
		}
	}
}

func TestBacklogCountsOnlyCoverTheBacklogsThePersonCanSee(t *testing.T) {
	f := newReadFixture(t)
	count := func(caller string) (int, map[string]int) {
		code, body := f.raw(caller, "GET", "/backlogs")
		if code != http.StatusOK {
			t.Fatalf("GET /backlogs as %q = %d %s", caller, code, body)
		}
		var list struct {
			Items []struct {
				ID     string `json:"id"`
				Issues int    `json:"issues"`
				Steps  []struct {
					Issues int `json:"issues"`
				} `json:"steps"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &list); err != nil {
			t.Fatal(err)
		}
		total, per := 0, map[string]int{}
		for _, b := range list.Items {
			per[b.ID] = b.Issues
			total += b.Issues
			inSteps := 0
			for _, s := range b.Steps {
				inSteps += s.Issues
			}
			if inSteps > b.Issues {
				t.Errorf("%q: backlog %s counts %d at its steps but %d in all", caller, b.ID, inSteps, b.Issues)
			}
		}
		return total, per
	}
	if total, per := count("dev"); total != 2 || len(per) != 1 || per["software"] != 2 {
		t.Errorf("dev counts %d in %v, want the two software issues only", total, per)
	}
	if total, per := count("owner"); total != 3 || per["backlog"] != 1 || per["software"] != 2 {
		t.Errorf("owner counts %d in %v", total, per)
	}
	if total, per := count("nobody"); total != 0 || len(per) != 0 {
		t.Errorf("nobody counts %d in %v", total, per)
	}
}
