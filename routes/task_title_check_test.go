package routes_test

import (
	"net/http"
	"testing"
)

func TestAnIssueTitleThatDrawsNothingIsRefused(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"owner": {"g-owner"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "g-owner"})
	h.expect(http.StatusCreated, "owner", "POST", "/backlogs", map[string]any{"name": "Software", "steps": []string{"To do", "Done"}})
	id := h.expect(http.StatusCreated, "owner", "POST", "/tasks", map[string]any{"title": "Kovács Anna kérése"})["id"].(string)

	for _, title := range []string{"ㅤ", "⠀", "ㅤㅤㅤ", "AnnaㅤPolgármester"} {
		h.expect(http.StatusBadRequest, "owner", "POST", "/tasks", map[string]any{"title": title})
		h.expect(http.StatusBadRequest, "owner", "PATCH", "/tasks/"+id, map[string]any{"title": title})
	}
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"title": "Őze Ödön: 2. rész"})
	h.expect(http.StatusOK, "owner", "PATCH", "/tasks/"+id, map[string]any{"priority": "high"})
}
