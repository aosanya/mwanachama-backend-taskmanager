package routes_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func snapshotStepCount(t *testing.T, body map[string]any, name string) int {
	t.Helper()
	for _, raw := range body["steps"].([]any) {
		step := raw.(map[string]any)
		if step["name"] == name {
			return int(step["count"].(float64))
		}
	}
	return -1
}

func TestBoardSnapshotCountsWhatPendingCountsAndNarrowsToTheCallersBacklogs(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"Open", "Shipped"}})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "DEV-001 in the first", "tags": []string{"Ready"}})
	moved := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "DEV-002 in software"})
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+moved["id"].(string)+"/move-to-step", map[string]any{"backlog": "software", "step": "open"})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})

	owner := h.expect(http.StatusOK, "owner", "GET", "/board-snapshot", nil)
	pending := h.expect(http.StatusOK, "owner", "GET", "/pending-tasks", nil)
	for _, raw := range pending["by_step"].([]any) {
		want := raw.(map[string]any)
		if got := snapshotStepCount(t, owner, want["name"].(string)); got != int(want["count"].(float64)) {
			t.Errorf("step %v: snapshot %d, pending %v", want["name"], got, want["count"])
		}
	}
	if int(owner["ready"].(float64)) != 1 {
		t.Errorf("owner ready = %v, want 1", owner["ready"])
	}
	if got := snapshotStepCount(t, h.expect(http.StatusOK, "dev", "GET", "/board-snapshot", nil), "Open"); got != 1 {
		t.Errorf("dev sees %d at Open, want 1", got)
	}
	if got := h.expect(http.StatusOK, "nobody", "GET", "/board-snapshot", nil)["steps"].([]any); len(got) != 0 {
		t.Errorf("nobody sees steps %v, want none", got)
	}
}

func TestBoardSnapshotRefusesAnUnknownOrUnseenBacklogAndANegativeThreshold(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"Open", "Shipped"}})

	if code, _ := h.call("owner", "GET", "/board-snapshot?backlog=nowhere", nil); code != http.StatusNotFound && code != http.StatusBadRequest {
		t.Errorf("unknown backlog answered %d, want a refusal", code)
	}
	if code, _ := h.call("dev", "GET", "/board-snapshot?backlog=software", nil); code != http.StatusNotFound && code != http.StatusBadRequest {
		t.Errorf("a backlog the caller cannot see answered %d, want a refusal", code)
	}
	if code, _ := h.call("owner", "GET", "/board-snapshot?stalled_days=-1", nil); code != http.StatusBadRequest {
		t.Errorf("negative stalled_days answered %d, want 400", code)
	}
}

func TestBoardSnapshotIsAnMCPToolNamedByItsAction(t *testing.T) {
	for _, tool := range routes.ToolShape() {
		if tool.Action != "taskmanager.task.board_snapshot" {
			continue
		}
		if tool.Name != "taskmanager_task_board_snapshot" {
			t.Fatalf("the tool is named %q", tool.Name)
		}
		for _, field := range []string{"backlog", "stalled_days"} {
			if !strings.Contains(string(tool.InputSchema), `"`+field+`"`) {
				t.Errorf("the input schema lacks %q: %s", field, tool.InputSchema)
			}
		}
		return
	}
	t.Fatal("no tool for taskmanager.task.board_snapshot")
}
