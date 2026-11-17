package routes_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func seedNextReadyHub(t *testing.T) *accessHub {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"In coming", "To do", "Doing"}})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Hidden", "steps": []string{"In coming", "To do", "Doing"}})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	for _, task := range []struct {
		backlog, step, code string
		rank                int
		tags                []string
		effort              string
	}{
		{"software", "to-do", "SW3", 300, []string{"Ready", "app"}, "high"},
		{"software", "to-do", "SW1", 100, []string{"Ready", "app", "backend"}, "low"},
		{"software", "to-do", "SW2", 200, []string{"Ready"}, "medium"},
		{"software", "to-do", "SW4", 50, []string{"Ready", "Sinbin"}, "low"},
		{"software", "to-do", "SW5", 60, []string{"Ready", "Blocked"}, "low"},
		{"software", "to-do", "SW6", 70, []string{"Waiting"}, "low"},
		{"software", "to-do", "SW7", 80, []string{"Ready", "Waiting"}, "low"},
		{"software", "in-coming", "SW8", 10, []string{"Ready"}, "low"},
		{"software", "doing", "SW9", 20, []string{"Ready"}, "low"},
		{"hidden", "to-do", "HD1", 5, []string{"Ready"}, "low"},
	} {
		made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{
			"title": "Title " + task.code, "code": task.code, "rank": task.rank, "tags": task.tags,
			"effort": task.effort, "description": "a long description of " + task.code,
		})
		h.expect(http.StatusOK, "owner", "POST", "/tasks/"+made["id"].(string)+"/move-to-step", map[string]any{"backlog": task.backlog, "step": task.step})
	}
	return h
}

func readyCodes(t *testing.T, body map[string]any) []string {
	t.Helper()
	codes := []string{}
	for _, item := range body["items"].([]any) {
		entry := item.(map[string]any)
		if _, has := entry["description"]; has {
			t.Errorf("an item carries a description: %v", entry)
		}
		codes = append(codes, entry["code"].(string))
	}
	return codes
}

func wantReady(t *testing.T, body map[string]any, total int, want ...string) {
	t.Helper()
	got := readyCodes(t, body)
	if strings.Join(got, ",") != strings.Join(want, ",") || int(body["total_ready"].(float64)) != total {
		t.Errorf("got %v total_ready %v, want %v total_ready %d", got, body["total_ready"], want, total)
	}
}

func TestNextReadyReturnsTheLowestRankFirstAndSkipsSinbinBlockedAndWaiting(t *testing.T) {
	h := seedNextReadyHub(t)

	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?backlog=software", nil), 3, "SW1")
	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?backlog=software&limit=5", nil), 3, "SW1", "SW2", "SW3")
	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?limit=5", nil), 4, "HD1", "SW1", "SW2", "SW3")
}

func TestNextReadyLimitDefaultsToOneAndStaysBetweenOneAndFive(t *testing.T) {
	h := seedNextReadyHub(t)

	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?backlog=software&limit=2", nil), 3, "SW1", "SW2")
	for _, limit := range []string{"0", "6", "-1"} {
		if status, _ := h.call("owner", "GET", "/next-ready?limit="+limit, nil); status != http.StatusBadRequest {
			t.Errorf("limit %s answered %d, want 400", limit, status)
		}
	}
}

func TestNextReadyNarrowsByTagAndByBacklog(t *testing.T) {
	h := seedNextReadyHub(t)

	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?tag=backend&limit=5", nil), 1, "SW1")
	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?tag=app&tag=backend&limit=5", nil), 1, "SW1")
	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?tag=app&limit=5", nil), 2, "SW1", "SW3")
	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?backlog=Hidden&limit=5", nil), 1, "HD1")
	wantReady(t, h.expect(http.StatusOK, "owner", "GET", "/next-ready?tag=nothing", nil), 0)
}

func TestNextReadyAnswersCompactItemsWithEffortAndNoDescription(t *testing.T) {
	h := seedNextReadyHub(t)

	body := h.expect(http.StatusOK, "owner", "GET", "/next-ready?backlog=software", nil)
	item := body["items"].([]any)[0].(map[string]any)
	want := map[string]any{"code": "SW1", "title": "Title SW1", "effort": "low", "backlog": "Software", "step": "To do", "rank": float64(100)}
	for key, value := range want {
		if item[key] != value {
			t.Errorf("%s is %v, want %v (%v)", key, item[key], value, item)
		}
	}
	for _, key := range []string{"id", "priority", "tags"} {
		if _, has := item[key]; !has {
			t.Errorf("the item has no %s: %v", key, item)
		}
	}
	if _, has := item["description"]; has {
		t.Errorf("the item carries a description: %v", item)
	}
}

func TestNextReadyShowsAPersonOnlyTheBacklogsTheirGroupsCanSee(t *testing.T) {
	h := seedNextReadyHub(t)

	wantReady(t, h.expect(http.StatusOK, "dev", "GET", "/next-ready?limit=5", nil), 3, "SW1", "SW2", "SW3")
	wantReady(t, h.expect(http.StatusOK, "nobody", "GET", "/next-ready?limit=5", nil), 0)
	for _, caller := range []string{"dev", "nobody"} {
		status, body := h.call(caller, "GET", "/next-ready?backlog=hidden", nil)
		if status != http.StatusBadRequest {
			t.Errorf("%s: an unseen backlog answered %d, want 400 (%v)", caller, status, body)
		}
	}
	if status, _ := h.call("owner", "GET", "/next-ready?backlog=nowhere", nil); status != http.StatusBadRequest {
		t.Errorf("an unknown backlog answered %d, want 400", status)
	}
}

func TestNextReadyIsAnMCPToolNamedByItsAction(t *testing.T) {
	for _, tool := range routes.ToolShape() {
		if tool.Action != "taskmanager.task.next_ready" {
			continue
		}
		if tool.Name != "taskmanager_task_next_ready" {
			t.Fatalf("the tool is named %q", tool.Name)
		}
		for _, field := range []string{"tag", "backlog", "limit"} {
			if !strings.Contains(string(tool.InputSchema), `"`+field+`"`) {
				t.Errorf("the input schema has no %s: %s", field, tool.InputSchema)
			}
		}
		if strings.Contains(string(tool.InputSchema), "requester_id") {
			t.Errorf("the caller is asked for in the input: %s", tool.InputSchema)
		}
		return
	}
	t.Fatal("no tool for taskmanager.task.next_ready")
}
