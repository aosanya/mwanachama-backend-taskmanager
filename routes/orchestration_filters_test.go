package routes_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func TestPendingTasksNarrowsByTheOrchestrationFiltersOverTheRoute(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "small held", "effort": "low", "assigned_to": "agent-1", "files": []string{"app/a/"}, "tags": []string{"Ready"}})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "big free", "effort": "high", "tags": []string{"Blocked"}})
	last := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "plain"})

	totals := map[string]int{
		"/pending-tasks":                                    3,
		"/pending-tasks?effort=low":                         1,
		"/pending-tasks?assignee=agent-1":                   1,
		"/pending-tasks?unassigned=true":                    2,
		"/pending-tasks?blocked=true":                       1,
		"/pending-tasks?blocked=false":                      2,
		"/pending-tasks?has_files=true":                     1,
		"/pending-tasks?has_files=false&unassigned=true":    2,
		"/pending-tasks?has_todos=true":                     0,
		"/pending-tasks?has_todos=false":                    3,
		"/pending-tasks?ids=" + last["id"].(string):         1,
		"/pending-tasks?created_since=2000-01-01T00:00:00Z": 3,
		"/pending-tasks?sort=priority":                      3,
	}
	for path, want := range totals {
		got := h.expect(http.StatusOK, "owner", "GET", path, nil)
		if int(got["total"].(float64)) != want {
			t.Errorf("%s: total %v, want %d", path, got["total"], want)
		}
	}
	for _, bad := range []string{"?effort=gigantic", "?sort=alphabetical", "?created_since=yesterday", "?assignee=a&unassigned=true", "?blocked=maybe"} {
		h.expect(http.StatusBadRequest, "owner", "GET", "/pending-tasks"+bad, nil)
	}
}

func TestNextReadyNarrowsByEffortAndRepoOverTheRoute(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "small", "effort": "low", "tags": []string{"Ready"}})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "big", "effort": "high", "tags": []string{"Ready"}})

	got := h.expect(http.StatusOK, "owner", "GET", "/next-ready?effort=high&limit=5", nil)
	if int(got["total_ready"].(float64)) != 1 {
		t.Fatalf("effort=high total_ready %v, want 1: %v", got["total_ready"], got)
	}
	h.expect(http.StatusBadRequest, "owner", "GET", "/next-ready?effort=gigantic", nil)
}

func TestTheOrchestrationFiltersAreInTheInputSchemaOfTheThreeTools(t *testing.T) {
	wanted := map[string][]string{
		"taskmanager.task.list":       {"assignee", "unassigned", "blocked", "waiting", "sinbin", "has_todos", "todos_open", "effort", "created_since", "updated_since", "has_files", "ids", "sort"},
		"taskmanager.task.pending":    {"assignee", "unassigned", "blocked", "waiting", "sinbin", "has_todos", "todos_open", "effort", "created_since", "updated_since", "has_files", "ids", "sort"},
		"taskmanager.task.next_ready": {"effort", "priority", "repo", "assignee", "unassigned", "has_todos", "todos_open", "has_files", "created_since", "updated_since"},
	}
	found := map[string]bool{}
	for _, tool := range routes.ToolShape() {
		fields, ok := wanted[tool.Action]
		if !ok {
			continue
		}
		found[tool.Action] = true
		for _, field := range fields {
			if !strings.Contains(string(tool.InputSchema), `"`+field+`"`) {
				t.Errorf("%s: the input schema has no %s: %s", tool.Action, field, tool.InputSchema)
			}
		}
	}
	if len(found) != len(wanted) {
		t.Fatalf("found tools %v, want %d", found, len(wanted))
	}
}
