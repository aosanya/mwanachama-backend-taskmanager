package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func (h *accessHub) listAs(caller, path string) []map[string]any {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), callerKey{}, caller))
	rec := httptest.NewRecorder()
	h.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("GET %s as %q = %d %s", path, caller, rec.Code, rec.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		h.t.Fatalf("GET %s: %v: %s", path, err, rec.Body.String())
	}
	return list
}

func TestAReaderAddsListsAndRemovesAReferenceAndAnUnseenIssueIsNotFound(t *testing.T) {
	f := newMoreWritesFixture(t)
	refs := "/tasks/" + f.issue + "/references"

	f.h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+f.issue+"/unassign", nil)
	added := f.h.expect(http.StatusCreated, "ann", "POST", refs, map[string]any{"kind": "file", "repo": "hub-steward", "target": "loader/importer.go"})
	if added["added_by"] != "ann" || added["source"] != "person" || added["confirmed"] != true || added["id"] == "" {
		t.Fatalf("got %v", added)
	}
	f.h.expect(http.StatusConflict, "ann", "POST", refs, map[string]any{"kind": "file", "repo": "hub-steward", "target": "loader/importer.go"})
	f.h.expect(http.StatusBadRequest, "ann", "POST", refs, map[string]any{"kind": "file", "target": "a.go"})

	list := f.h.listAs("boss", refs)
	if len(list) != 1 || list[0]["target"] != "loader/importer.go" {
		t.Fatalf("got %v", list)
	}

	f.h.expect(http.StatusNotFound, "nog", "GET", refs, nil)
	f.h.expect(http.StatusNotFound, "nog", "POST", refs, map[string]any{"kind": "link", "target": "https://example.test"})
	f.h.expect(http.StatusNotFound, "nog", "DELETE", refs+"/"+added["id"].(string), nil)

	f.h.expect(http.StatusNoContent, "ann", "DELETE", refs+"/"+added["id"].(string), nil)
	f.h.expect(http.StatusNotFound, "ann", "DELETE", refs+"/"+added["id"].(string), nil)
	if left := f.h.listAs("boss", refs); len(left) != 0 {
		t.Fatalf("got %v", left)
	}
}

func TestAnAssistantProposesAndAPersonConfirmsAReferenceOverTheRoutes(t *testing.T) {
	f := newMoreWritesFixture(t)
	refs := "/tasks/" + f.issue + "/references"

	proposed := f.h.expect(http.StatusCreated, "ann", "POST", refs+"/proposals", map[string]any{"kind": "folder", "repo": "hub-steward", "target": "code/app"})
	if proposed["source"] != "assistant" || proposed["confirmed"] != false {
		t.Fatalf("got %v", proposed)
	}
	f.h.expect(http.StatusConflict, "ann", "POST", refs+"/proposals", map[string]any{"kind": "folder", "repo": "hub-steward", "target": "code/app"})
	f.h.expect(http.StatusBadRequest, "ann", "POST", refs+"/proposals", map[string]any{"kind": "folder", "target": "code/app"})

	confirmed := f.h.expect(http.StatusOK, "ann", "POST", refs+"/"+proposed["id"].(string)+"/confirm", nil)
	if confirmed["confirmed"] != true || confirmed["source"] != "assistant" {
		t.Fatalf("got %v", confirmed)
	}
	f.h.expect(http.StatusNotFound, "ann", "POST", refs+"/nothing/confirm", nil)
	f.h.expect(http.StatusNotFound, "nog", "POST", refs+"/"+proposed["id"].(string)+"/confirm", nil)
	f.h.expect(http.StatusNotFound, "nog", "POST", refs+"/proposals", map[string]any{"kind": "link", "target": "https://example.test"})
}
