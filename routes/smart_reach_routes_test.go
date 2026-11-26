package routes_test

import (
	"net/http"
	"testing"
)

func TestSettingTheRulesOfASmartBacklogAnswersWithTheCallersOwnCount(t *testing.T) {
	h := newAccessHub(t, map[string][]string{"eve": {"g-emea"}, "ada": {"g-apac"}, "boss": {"owners"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	for _, area := range []string{"g-emea", "g-apac"} {
		made := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": area, "steps": []string{"Open"}})["id"].(string)
		h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+made+"/access/"+area, map[string]string{"level": "see"})
		h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "sprint work in " + area, "backlog": made, "step": "open"})
	}
	titled := func(word string) map[string]any {
		return map[string]any{"match": "all", "rules": []map[string]string{{"field": "title", "operator": "contains", "value": word}}}
	}
	smart := h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Sprint", "rules": titled("sprint")})["id"].(string)

	h.expect(http.StatusForbidden, "eve", "PUT", "/backlogs/"+smart+"/rules", map[string]any{"rules": titled("sprint work")})
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+smart+"/access/g-emea", map[string]string{"level": "edit"})

	for caller, want := range map[string]float64{"eve": 1, "boss": 2} {
		got := h.expect(http.StatusOK, caller, "PUT", "/backlogs/"+smart+"/rules", map[string]any{"rules": titled("sprint work")})
		if got["issues"] != want {
			t.Fatalf("%s was answered %v issues, want %v", caller, got["issues"], want)
		}
	}
}
