package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func (h *accessHub) titlesAs(caller string) []string {
	h.t.Helper()
	req := httptest.NewRequest("GET", "/tasks", nil)
	req = req.WithContext(context.WithValue(req.Context(), callerKey{}, caller))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("GET /tasks as %q = %d %s", caller, rec.Code, rec.Body.String())
	}
	var tasks []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &tasks); err != nil {
		h.t.Fatalf("GET /tasks body %s: %v", rec.Body.String(), err)
	}
	titles := []string{}
	for _, task := range tasks {
		titles = append(titles, task["title"].(string))
	}
	return titles
}

func TestListTasksShowsAPersonOnlyTheIssuesOfTheBacklogsTheirGroupsCanSee(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"Open", "Shipped"}})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "in the first backlog"})
	moved := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "in software"})
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+moved["id"].(string)+"/move-to-step", map[string]any{"backlog": "software", "step": "open"})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})

	cases := map[string]int{"owner": 2, "dev": 1, "nobody": 0, "": 2}
	for person, want := range cases {
		if got := h.titlesAs(person); len(got) != want {
			t.Errorf("%q lists %v, want %d", person, got, want)
		}
	}
	if got := h.titlesAs("dev"); got[0] != "in software" {
		t.Errorf("dev lists %v", got)
	}
}
