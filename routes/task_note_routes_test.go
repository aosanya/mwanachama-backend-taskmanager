package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func (h *accessHub) notesAs(caller, path string) []map[string]any {
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

func TestACanSeeReaderAddsANoteToTheLogAndEveryoneWhoSeesTheIssueReadsItInOrder(t *testing.T) {
	f := newMoreWritesFixture(t)
	notes := "/tasks/" + f.issue + "/notes"

	f.h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+f.issue+"/unassign", nil)
	first := f.h.expect(http.StatusCreated, "boss", "POST", notes, map[string]any{"kind": "worker", "text": "Took the issue.", "links": []string{"commit 3f9a1c2"}})
	if first["author_id"] != "boss" || first["kind"] != "worker" || first["created_at"] == "" {
		t.Fatalf("got %v", first)
	}
	second := f.h.expect(http.StatusCreated, "ann", "POST", notes, map[string]any{"text": "Checked on a phone."})
	if second["author_id"] != "ann" || second["kind"] != "person" {
		t.Fatalf("got %v", second)
	}
	list := f.h.notesAs("ann", notes)
	if len(list) != 2 || list[0]["text"] != "Took the issue." || list[1]["text"] != "Checked on a phone." {
		t.Fatalf("got %v", list)
	}
}

func TestAPersonInNoGroupCannotReadOrAddToTheLogOfAnIssueTheyCannotSee(t *testing.T) {
	f := newMoreWritesFixture(t)
	notes := "/tasks/" + f.issue + "/notes"

	f.h.expect(http.StatusNotFound, "nog", "POST", notes, map[string]any{"text": "x"})
	f.h.expect(http.StatusNotFound, "nog", "GET", notes, nil)
}

func TestABlankNoteIsRefused(t *testing.T) {
	f := newMoreWritesFixture(t)

	f.h.expect(http.StatusBadRequest, "ann", "POST", "/tasks/"+f.issue+"/notes", map[string]any{"text": "  "})
}
