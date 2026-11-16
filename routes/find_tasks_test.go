package routes_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

func seedFindHub(t *testing.T) (*accessHub, map[string]string) {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}, "dev": {"g-dev"}, "nobody": nil})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"Open", "Shipped"}})
	ids := map[string]string{}
	for _, task := range []map[string]any{
		{"title": "Login button", "code": "SW10", "description": "sits in the header", "tags": []string{"app"}},
		{"title": "Login footer", "code": "SW11"},
		{"title": "Software only", "code": "SW12", "description": "reachable by dev"},
	} {
		made := h.expect(http.StatusCreated, "owner", "POST", "/tasks", task)
		ids[task["code"].(string)] = made["id"].(string)
	}
	h.expect(http.StatusOK, "owner", "POST", "/tasks/"+ids["SW12"]+"/move-to-step", map[string]any{"backlog": "software", "step": "open"})
	h.expect(http.StatusOK, "owner", "PUT", "/backlogs/software/access/g-dev", map[string]string{"level": "see"})
	return h, ids
}

func findPath(q string, extra ...string) string {
	return "/find-tasks?q=" + url.QueryEscape(q) + strings.Join(extra, "")
}

func codesOf(t *testing.T, body map[string]any) []string {
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

func wantCodes(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	got := codesOf(t, body)
	if strings.Join(got, ",") != strings.Join(want, ",") || int(body["total"].(float64)) != len(want) {
		t.Errorf("got %v total %v, want %v", got, body["total"], want)
	}
}

func TestFindTasksAnswersAnIdACodeAndWordsThroughTheRouter(t *testing.T) {
	h, ids := seedFindHub(t)

	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath(ids["SW11"]), nil), "SW11")
	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath("sw11"), nil), "SW11")
	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath("SW1"), nil), "SW10", "SW11", "SW12")
	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath("header"), nil), "SW10")
	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath("login app"), nil), "SW10")
}

func TestFindTasksNarrowsByBacklogNamedByNameAndByIdAndPages(t *testing.T) {
	h, _ := seedFindHub(t)

	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath("SW1", "&backlog=Software"), nil), "SW12")
	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath("SW1", "&backlog=software&step=open"), nil), "SW12")
	page := h.expect(http.StatusOK, "owner", "GET", findPath("SW1", "&limit=1&offset=1"), nil)
	if len(page["items"].([]any)) != 1 || int(page["total"].(float64)) != 3 {
		t.Errorf("page %v", page)
	}
}

func TestFindTasksShowsAPersonOnlyTheBacklogsTheirGroupsCanSee(t *testing.T) {
	h, ids := seedFindHub(t)

	wantCodes(t, h.expect(http.StatusOK, "dev", "GET", findPath("SW1"), nil), "SW12")
	wantCodes(t, h.expect(http.StatusOK, "dev", "GET", findPath(ids["SW10"]), nil))
	wantCodes(t, h.expect(http.StatusOK, "nobody", "GET", findPath("SW1"), nil))
}

func TestFindTasksFindsAClosedIssueOnlyWhenAsked(t *testing.T) {
	h, ids := seedFindHub(t)
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+ids["SW11"], map[string]any{"status": "in_progress"})
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+ids["SW11"], map[string]any{"status": "completed"})

	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath("footer"), nil))
	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath("footer", "&include_closed=true"), nil), "SW11")
	wantCodes(t, h.expect(http.StatusOK, "owner", "GET", findPath(ids["SW11"], "&include_closed=true"), nil), "SW11")
}

func TestFindTasksRefusesABlankQueryWithAMessage(t *testing.T) {
	h, _ := seedFindHub(t)

	for _, path := range []string{"/find-tasks", "/find-tasks?q=", "/find-tasks?q=%20%20"} {
		status, body := h.call("owner", "GET", path, nil)
		if status != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400", path, status)
		}
		if message, _ := body["error"].(string); !strings.Contains(message, "q") {
			t.Errorf("%s gave no message naming q: %v", path, body)
		}
	}
}

func TestFindTasksIsAnMCPToolNamedByItsAction(t *testing.T) {
	for _, tool := range routes.ToolShape() {
		if tool.Action != "taskmanager.task.find" {
			continue
		}
		if tool.Name != "taskmanager_task_find" {
			t.Fatalf("the tool is named %q", tool.Name)
		}
		for _, field := range []string{"tag", "backlog", "step", "q", "include_closed", "limit", "offset"} {
			if !strings.Contains(string(tool.InputSchema), `"`+field+`"`) {
				t.Errorf("the input schema has no %s: %s", field, tool.InputSchema)
			}
		}
		if strings.Contains(string(tool.InputSchema), "requester_id") {
			t.Errorf("the caller is asked for in the input: %s", tool.InputSchema)
		}
		return
	}
	t.Fatal("no tool for taskmanager.task.find")
}
