package routes_test

import (
	"net/http"
	"testing"
)

func TestACreatedIssueComesBackWithItsCodeAndAnOverwriteIsKept(t *testing.T) {
	f := newMoreWritesFixture(t)

	first := f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "a", "project_name": "swe", "code": "SW40"})
	second := f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "b", "project_name": "swe"})

	if first["code"] != "SW40" || second["code"] != "SW41" || second["code_series"] != "SW" {
		t.Fatalf("got %v and %v, want SW40 then SW41", first, second)
	}
	read := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+second["id"].(string), nil)
	if read["code"] != "SW41" {
		t.Fatalf("read back %v", read)
	}
}

func TestACodeAnotherIssueOfTheProjectHoldsIsRefusedWithAConflict(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "a", "project_name": "swe", "code": "SW40"})

	f.h.expect(http.StatusConflict, "boss", "POST", "/tasks", map[string]any{"title": "b", "project_name": "swe", "code": "SW40"})
}
