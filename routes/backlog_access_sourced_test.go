package routes_test

import (
	"net/http"
	"testing"
)

func TestSourcedTasksAreRefusedToAPersonWithoutCanMoveOnTheFirstBacklog(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"boss": {"owners"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	body := map[string]any{
		"source": "feed",
		"items":  []map[string]any{{"key": "k1", "title": "from a feed", "due_at": "2030-01-01T00:00:00Z"}},
	}
	h.expect(http.StatusForbidden, "nog", "POST", "/sourced-tasks", body)
	opened := h.expect(http.StatusOK, "boss", "POST", "/sourced-tasks", body)
	if items := opened["items"].([]any); len(items) != 1 {
		t.Fatalf("sourced tasks opened %v", opened)
	}
}

func TestGeneratedTasksAreRefusedToAPersonWithoutCanMoveOnTheFirstBacklog(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"boss": {"owners"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	body := map[string]any{"run": "r1", "template": map[string]any{"title": "per subject"}, "subjects": []string{"s1"}}
	h.expect(http.StatusForbidden, "nog", "POST", "/generated-tasks", body)
	h.expect(http.StatusOK, "boss", "POST", "/generated-tasks", body)
}
