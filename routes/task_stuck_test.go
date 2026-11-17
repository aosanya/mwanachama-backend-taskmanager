package routes_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

type stuckHub struct {
	*claimNextHub
}

func newStuckHub(t *testing.T) *stuckHub {
	t.Helper()
	return &stuckHub{newClaimNextHub(t)}
}

func (h *stuckHub) exec(query string, args ...any) {
	h.t.Helper()
	db, err := gorm.Open(sqlite.Open(h.dsn), &gorm.Config{})
	if err != nil {
		h.t.Fatal(err)
	}
	if err := db.Exec(query, args...).Error; err != nil {
		h.t.Fatal(err)
	}
}

func (h *stuckHub) age(id string, days int) {
	h.t.Helper()
	then := time.Now().UTC().AddDate(0, 0, -days).Format(time.RFC3339)
	h.exec("UPDATE "+specTable(h.t, "work_item")+" SET updated_at = ? WHERE id = ?", then, id)
	h.exec("UPDATE "+specTable(h.t, "step")+" SET updated_at = ? WHERE parent_task_id = ?", then, id)
	h.exec("UPDATE "+specTable(h.t, "task_note")+" SET created_at = ? WHERE task_id = ?", then, id)
}

func (h *stuckHub) todo(issue, title string, waitsOn ...string) string {
	h.t.Helper()
	body := map[string]any{"parent_task_id": issue, "title": title, "description": "does " + title, "instructions": "do it"}
	if len(waitsOn) > 0 {
		body["depends_on"] = waitsOn
	}
	return h.expect(http.StatusCreated, "owner", "POST", "/todos", body)["id"].(string)
}

func (h *stuckHub) setTodo(id, status string, extra map[string]any) {
	h.t.Helper()
	body := map[string]any{"status": status}
	for k, v := range extra {
		body[k] = v
	}
	h.expect(http.StatusOK, "owner", "PUT", "/todos/"+id+"/status", body)
}

func (h *stuckHub) claimed(title string, rank int, files ...string) string {
	h.t.Helper()
	id := h.issueAt("in-progress", title, rank, files)
	h.exec("UPDATE "+specTable(h.t, "work_item")+" SET assigned_to = ? WHERE id = ?", "worker", id)
	return id
}

func (h *stuckHub) healthy(title string, rank int, files ...string) string {
	h.t.Helper()
	id := h.claimed(title, rank, files...)
	first := h.todo(id, "Build "+title)
	h.todo(id, "Documentation: "+title, first)
	h.setTodo(first, "In progress", nil)
	return id
}

func stuckItem(t *testing.T, body map[string]any, id string) map[string]any {
	t.Helper()
	for _, raw := range body["items"].([]any) {
		item := raw.(map[string]any)
		if item["id"] == id {
			return item
		}
	}
	return nil
}

func stuckReasons(item map[string]any) string {
	reasons := []string{}
	for _, raw := range item["reasons"].([]any) {
		reasons = append(reasons, raw.(string))
	}
	sort.Strings(reasons)
	return strings.Join(reasons, ",")
}

func (h *stuckHub) stuck(query string) map[string]any {
	h.t.Helper()
	return h.expect(http.StatusOK, "owner", "GET", "/stuck"+query, nil)
}

func TestStuckFlagsAnInProgressIssueWithNoTodosAndNoAssignee(t *testing.T) {
	h := newStuckHub(t)
	id := h.issueAt("in-progress", "Empty", 10, nil)

	item := stuckItem(t, h.stuck(""), id)

	if item == nil {
		t.Fatal("the issue with no todos and no assignee is not listed")
	}
	if got := stuckReasons(item); got != "no_todos,unassigned_in_progress" {
		t.Errorf("reasons %q, want no_todos,unassigned_in_progress", got)
	}
	if item["todos_total"].(float64) != 0 || item["todos_done"].(float64) != 0 {
		t.Errorf("todos %v of %v, want 0 of 0", item["todos_done"], item["todos_total"])
	}
	if !strings.Contains(item["next_action"].(string), "todo") {
		t.Errorf("next_action %q should say to write todos", item["next_action"])
	}
}

func TestStuckLeavesOutAnIssueWithAStartedTodoAnAssigneeAndFreshActivity(t *testing.T) {
	h := newStuckHub(t)
	id := h.healthy("Moving", 10)

	body := h.stuck("")

	if stuckItem(t, body, id) != nil {
		t.Errorf("a healthy issue is listed: %v", body)
	}
	if body["total"].(float64) != 0 {
		t.Errorf("total %v, want 0", body["total"])
	}
}

func TestStuckFlagsAnIssueWhoseTodosWereNeverStarted(t *testing.T) {
	h := newStuckHub(t)
	id := h.claimed("Idle", 10)
	h.todo(id, "Build it")
	h.todo(id, "Other part")

	item := stuckItem(t, h.stuck(""), id)

	if item == nil || stuckReasons(item) != "no_todo_started" {
		t.Fatalf("item %v, want only no_todo_started", item)
	}
	if item["assignee"] != "worker" {
		t.Errorf("assignee %v, want worker", item["assignee"])
	}
}

func TestStuckFlagsAnIssueWhoseRemainingTodosAreAllBlocked(t *testing.T) {
	h := newStuckHub(t)
	id := h.claimed("Waiting", 10)
	first := h.todo(id, "Build it")
	h.todo(id, "Then this", first)
	h.setTodo(first, "Blocked", map[string]any{"blocked_reason": "waiting for the other repo"})

	item := stuckItem(t, h.stuck(""), id)

	if item == nil || !strings.Contains(stuckReasons(item), "all_remaining_todos_blocked") {
		t.Fatalf("item %v, want all_remaining_todos_blocked", item)
	}
}

func TestStuckFlagsAnIssueWithOnlyDocumentationLeftOpen(t *testing.T) {
	h := newStuckHub(t)
	id := h.claimed("Nearly there", 10)
	first := h.todo(id, "Build it")
	h.todo(id, "Documentation: one row", first)
	h.setTodo(first, "In progress", nil)
	h.setTodo(first, "Done", nil)

	item := stuckItem(t, h.stuck(""), id)

	if item == nil || stuckReasons(item) != "only_documentation_left_open" {
		t.Fatalf("item %v, want only only_documentation_left_open", item)
	}
	if item["todos_done"].(float64) != 1 || item["todos_total"].(float64) != 2 {
		t.Errorf("todos %v of %v, want 1 of 2", item["todos_done"], item["todos_total"])
	}
}

func TestStuckNoActivityHonoursStalledDays(t *testing.T) {
	h := newStuckHub(t)
	id := h.healthy("Quiet", 10)
	h.age(id, 3)

	if item := stuckItem(t, h.stuck(""), id); item == nil || stuckReasons(item) != "no_activity" || item["days_since_activity"].(float64) != 3 {
		t.Fatalf("default threshold: %v, want no_activity at 3 days", item)
	}
	if item := stuckItem(t, h.stuck("?stalled_days=5"), id); item != nil {
		t.Errorf("a 5-day threshold lists the 3-day-old issue: %v", item)
	}
	if item := stuckItem(t, h.stuck("?stalled_days=1"), id); item == nil {
		t.Errorf("a 1-day threshold does not list the 3-day-old issue")
	}
}

func TestStuckANoteCountsAsActivity(t *testing.T) {
	h := newStuckHub(t)
	id := h.healthy("Quiet but noting", 10)
	h.age(id, 3)
	h.expect(http.StatusCreated, "mover", "POST", "/tasks/"+id+"/notes", map[string]any{"text": "halfway through the tests"})

	if item := stuckItem(t, h.stuck(""), id); item != nil {
		t.Errorf("an issue with a note from just now is listed: %v", item)
	}
}

func TestStuckAnExpiredLeaseIsFlagged(t *testing.T) {
	h := newStuckHub(t)
	id := h.ready("Abandoned", 10)
	h.expect(http.StatusOK, "mover", "POST", "/claim-next", map[string]any{"agent": "w1", "lease_minutes": 5})
	first := h.todo(id, "Build it")
	h.setTodo(first, "In progress", nil)
	if item := stuckItem(t, h.stuck(""), id); item != nil {
		t.Fatalf("an issue under a live lease is listed: %v", item)
	}
	h.expireLease(id)

	item := stuckItem(t, h.stuck(""), id)

	if item == nil || stuckReasons(item) != "lease_expired" {
		t.Fatalf("item %v, want only lease_expired", item)
	}
}

func TestStuckFlagsTwoInProgressIssuesThatTouchTheSameFiles(t *testing.T) {
	h := newStuckHub(t)
	a := h.healthy("First", 10, "code/api/task.go")
	b := h.healthy("Second", 20, "code/api/task.go")
	c := h.healthy("Third", 30, "code/other/file.go")

	body := h.stuck("")

	for _, id := range []string{a, b} {
		if item := stuckItem(t, body, id); item == nil || stuckReasons(item) != "files_clash_with_another_in_progress" {
			t.Errorf("item %v, want only files_clash_with_another_in_progress", item)
		}
	}
	if stuckItem(t, body, c) != nil {
		t.Errorf("the issue with its own files is listed")
	}
}

func TestStuckListsTheMostReasonsFirstThenTheOldestQuiet(t *testing.T) {
	h := newStuckHub(t)
	oneReasonOlder := h.healthy("Older", 10)
	h.age(oneReasonOlder, 6)
	oneReasonNewer := h.healthy("Newer", 20)
	h.age(oneReasonNewer, 3)
	twoReasons := h.issueAt("in-progress", "Empty", 30, nil)

	body := h.stuck("")

	got := []string{}
	for _, raw := range body["items"].([]any) {
		got = append(got, raw.(map[string]any)["id"].(string))
	}
	want := []string{twoReasons, oneReasonOlder, oneReasonNewer}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order %v, want %v", got, want)
	}
	if body["total"].(float64) != 3 {
		t.Errorf("total %v, want 3", body["total"])
	}
}

func TestStuckNeverListsAClosedIssueOrOneOutsideTheInProgressStep(t *testing.T) {
	h := newStuckHub(t)
	closed := h.issueAt("in-progress", "Closed", 10, nil)
	if h.expect(http.StatusOK, "owner", "GET", "/tasks/"+closed, nil)["status"] != "in_progress" {
		h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+closed, map[string]any{"status": "in_progress"})
	}
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+closed, map[string]any{"status": "completed"})
	waiting := h.ready("Not started", 20)
	review := h.issueAt("review", "In review", 30, nil)

	body := h.stuck("")

	for _, id := range []string{closed, waiting, review} {
		if stuckItem(t, body, id) != nil {
			t.Errorf("issue %s is listed", id)
		}
	}
}

func TestStuckReasonFilterKeepsOnlyThatRule(t *testing.T) {
	h := newStuckHub(t)
	empty := h.issueAt("in-progress", "Empty", 10, nil)
	idle := h.claimed("Idle", 20)
	h.todo(idle, "Build it")

	body := h.stuck("?reason=no_todo_started")

	if stuckItem(t, body, idle) == nil || stuckItem(t, body, empty) != nil || body["total"].(float64) != 1 {
		t.Errorf("reason filter answered %v, want only the idle issue", body)
	}
}

func TestStuckPagesWithLimitAndOffset(t *testing.T) {
	h := newStuckHub(t)
	for i := 0; i < 3; i++ {
		h.issueAt("in-progress", "Empty", 10*(i+1), nil)
	}

	first := h.stuck("?limit=2")
	second := h.stuck("?limit=2&offset=2")

	if len(first["items"].([]any)) != 2 || len(second["items"].([]any)) != 1 || first["total"].(float64) != 3 {
		t.Errorf("paging answered %v then %v, want 2 then 1 of 3", first, second)
	}
}

func TestStuckRefusesBadArguments(t *testing.T) {
	h := newStuckHub(t)
	for _, query := range []string{"?limit=101", "?limit=-1", "?stalled_days=-1", "?reason=nonsense", "?backlog=nowhere"} {
		if code, body := h.call("owner", "GET", "/stuck"+query, nil); code != http.StatusBadRequest && code != http.StatusNotFound {
			t.Errorf("%s answered %d %v, want a refusal", query, code, body)
		}
	}
	if code, _ := h.call("owner", "GET", "/stuck?limit=101", nil); code != http.StatusBadRequest {
		t.Errorf("limit above 100 did not answer 400")
	}
	if code, _ := h.call("owner", "GET", "/stuck?reason=nonsense", nil); code != http.StatusBadRequest {
		t.Errorf("an unknown reason did not answer 400")
	}
}

func TestStuckNarrowsToABacklogAndRefusesOneTheCallerCannotSee(t *testing.T) {
	h := newStuckHub(t)
	id := h.issueAt("in-progress", "Empty", 10, nil)

	if stuckItem(t, h.stuck("?backlog=journey-book"), id) == nil {
		t.Errorf("the backlog filter lost the issue")
	}
	if code, _ := h.call("nobody", "GET", "/stuck?backlog=journey-book", nil); code != http.StatusBadRequest && code != http.StatusNotFound {
		t.Errorf("a backlog the caller cannot see answered %d, want a refusal", code)
	}
	if got := h.expect(http.StatusOK, "nobody", "GET", "/stuck", nil)["items"].([]any); len(got) != 0 {
		t.Errorf("a caller who sees nothing got %v", got)
	}
}

func TestStuckIsAnMCPToolNamedByItsAction(t *testing.T) {
	for _, tool := range routes.ToolShape() {
		if tool.Action != "taskmanager.task.stuck" {
			continue
		}
		if tool.Name != "taskmanager_task_stuck" {
			t.Fatalf("the tool is named %q", tool.Name)
		}
		for _, field := range []string{"backlog", "tag", "stalled_days", "reason", "limit", "offset"} {
			if !strings.Contains(string(tool.InputSchema), `"`+field+`"`) {
				t.Errorf("the input schema has no %s: %s", field, tool.InputSchema)
			}
		}
		if strings.Contains(string(tool.InputSchema), "requester_id") {
			t.Errorf("the caller is asked for in the input: %s", tool.InputSchema)
		}
		return
	}
	t.Fatal("no tool for taskmanager.task.stuck")
}
