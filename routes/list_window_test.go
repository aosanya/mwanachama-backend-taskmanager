package routes_test

import (
	"net/http"
	"testing"
)

func TestTheTaskListRefusesAWindowThatCannotBeAPage(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "one"})
	for _, rawQuery := range []string{"limit=0", "limit=-1", "offset=-1", "limit=1&offset=-3"} {
		if status, _ := h.call("owner", "GET", "/tasks?"+rawQuery, nil); status != http.StatusBadRequest {
			t.Errorf("GET /tasks?%s answered %d, want 400", rawQuery, status)
		}
	}
}

func TestTheTaskListAnswersAWindowThatCanBeAPage(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}})
	h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "one"})
	for _, rawQuery := range []string{"", "limit=1", "limit=25&offset=0", "offset=1"} {
		path := "/tasks"
		if rawQuery != "" {
			path += "?" + rawQuery
		}
		if status, _ := h.call("owner", "GET", path, nil); status != http.StatusOK {
			t.Errorf("GET %s answered %d, want 200", path, status)
		}
	}
}
