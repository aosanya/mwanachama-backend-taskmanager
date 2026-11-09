package routes_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func TestPendingTasksAnswersInOneCallAndNarrowsToTheCallersBacklogs(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"Open", "Shipped"}})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "DEV-001 in the first", "description": "a long description"})
	moved := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "DEV-002 in software"})
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+moved["id"].(string)+"/move-to-step", map[string]any{"backlog": "software", "step": "open"})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})

	for person, want := range map[string]int{"owner": 2, "dev": 1, "nobody": 0, "": 2} {
		got := h.expect(http.StatusOK, person, "GET", "/pending-tasks", nil)
		if int(got["total"].(float64)) != want {
			t.Errorf("%q: total %v, want %d (%v)", person, got["total"], want, got)
		}
		items := got["items"].([]any)
		if len(items) != want {
			t.Errorf("%q: %d items, want %d", person, len(items), want)
		}
		for _, item := range items {
			if _, has := item.(map[string]any)["description"]; has {
				t.Errorf("%q: an item carries a description: %v", person, item)
			}
		}
	}
	filtered := h.expect(http.StatusOK, "owner", "GET", "/pending-tasks?q=software&limit=1", nil)
	if int(filtered["total"].(float64)) != 1 {
		t.Errorf("q=software total %v", filtered["total"])
	}
}

func TestPendingTasksIsAnMCPToolNamedByItsAction(t *testing.T) {
	for _, tool := range routes.ToolShape() {
		if tool.Action != "taskmanager.task.pending" {
			continue
		}
		if tool.Name != "taskmanager_task_pending" {
			t.Fatalf("the tool is named %q", tool.Name)
		}
		for _, field := range []string{"tag", "backlog", "step", "priority", "q", "limit", "offset"} {
			if !strings.Contains(string(tool.InputSchema), `"`+field+`"`) {
				t.Errorf("the input schema has no %s: %s", field, tool.InputSchema)
			}
		}
		if strings.Contains(string(tool.InputSchema), "requester_id") {
			t.Errorf("the caller is asked for in the input: %s", tool.InputSchema)
		}
		return
	}
	t.Fatal("no tool for taskmanager.task.pending")
}
