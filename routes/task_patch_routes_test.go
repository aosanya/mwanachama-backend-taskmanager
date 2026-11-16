package routes_test

import (
	"net/http"
	"testing"
)

func TestPatchingOnlyTheDescriptionKeepsTagsProjectPriorityAndRank(t *testing.T) {
	f := newMoreWritesFixture(t)
	made := f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{
		"title": "HM1", "description": "old", "priority": "critical", "tags": []string{"hungarian-municipality", "Ready"},
		"project_name": "projects/hungarian-municipality", "rank": 500, "backlog": f.docs, "step": "write",
	})
	path := "/tasks/" + made["id"].(string)

	patched := f.h.expect(http.StatusOK, "boss", "PATCH", path, map[string]any{"description": "new"})
	read := f.h.expect(http.StatusOK, "boss", "GET", path, nil)
	for _, got := range []map[string]any{patched, read} {
		if got["description"] != "new" || got["title"] != "HM1" || got["priority"] != "critical" ||
			got["project_name"] != "projects/hungarian-municipality" || got["rank"] != float64(500) ||
			got["backlog"] != f.docs || got["step"] != "write" {
			t.Fatalf("got %v", got)
		}
		if tags, _ := got["tags"].([]any); len(tags) != 2 {
			t.Fatalf("tags %v", got["tags"])
		}
	}
}

func TestPatchRefusesAFieldItDoesNotKnowAndAnIssueThatIsNotThere(t *testing.T) {
	f := newMoreWritesFixture(t)

	f.h.expect(http.StatusBadRequest, "boss", "PATCH", "/tasks/"+f.issue, map[string]any{"rank": 1})
	f.h.expect(http.StatusNotFound, "boss", "PATCH", "/tasks/nope", map[string]any{"title": "x"})
}
