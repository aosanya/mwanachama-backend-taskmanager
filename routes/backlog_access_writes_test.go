package routes_test

import (
	"net/http"
	"testing"
)

func TestCanSeeDoesNotAllowWritingThroughTheRoutes(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"ann": {"contributors"}, "boss": {"owners"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	docs := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write", "Done"}})["id"].(string)
	issue := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "write the guide", "backlog": docs, "step": "write"})["id"].(string)
	other := h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "second", "backlog": docs, "step": "write"})["id"].(string)
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/contributors", map[string]string{"level": "see"})

	h.expect(http.StatusForbidden, "ann", "POST", "/tasks", map[string]any{"title": "new", "backlog": docs, "step": "write"})
	h.expect(http.StatusForbidden, "ann", "PUT", "/tasks/"+issue, map[string]any{"title": "renamed"})
	h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+issue+"/move", map[string]string{"state": "Done"})
	h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+issue+"/place", map[string]string{"after": other})
	h.expect(http.StatusForbidden, "ann", "DELETE", "/tasks/"+issue, nil)
	h.expect(http.StatusForbidden, "ann", "POST", "/relationships", map[string]string{"label": "depends_on", "from_id": issue, "to_id": other})

	h.expect(http.StatusOK, "boss", "GET", "/tasks/"+issue, nil)
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+docs+"/access/contributors", map[string]string{"level": "move"})
	h.expect(http.StatusOK, "ann", "PUT", "/tasks/"+issue, map[string]any{"title": "renamed", "status": "pending", "backlog": docs, "step": "write"})
	h.expect(http.StatusNoContent, "ann", "DELETE", "/tasks/"+other, nil)
}
