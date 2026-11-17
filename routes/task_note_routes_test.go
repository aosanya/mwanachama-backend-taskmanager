package routes_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	return withoutChangeEntries(list)
}

func withoutChangeEntries(list []map[string]any) []map[string]any {
	kept := []map[string]any{}
	for _, note := range list {
		text, _ := note["text"].(string)
		if note["kind"] == "automatic" && note["automatic"] == true && isChangeEntryText(text) {
			continue
		}
		kept = append(kept, note)
	}
	return kept
}

func isChangeEntryText(text string) bool {
	if text == "Deleted" {
		return true
	}
	for _, label := range []string{"Moved ", "Assignee ", "Status ", "Tags "} {
		if strings.HasPrefix(text, label) && strings.Contains(text, " to ") {
			return true
		}
	}
	return false
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

func (f *moreWritesFixture) sixNotes() []string {
	f.h.t.Helper()
	notes := "/tasks/" + f.issue + "/notes"
	kinds := []string{"person", "worker", "person", "worker", "person", "worker"}
	ids := make([]string, 0, len(kinds))
	for i, kind := range kinds {
		made := f.h.expect(http.StatusCreated, "boss", "POST", notes, map[string]any{"kind": kind, "text": "entry " + string(rune('1'+i))})
		ids = append(ids, made["id"].(string))
	}
	return ids
}

func noteTexts(list []map[string]any) []string {
	texts := make([]string, 0, len(list))
	for _, note := range list {
		texts = append(texts, note["text"].(string))
	}
	return texts
}

func TestTheLogWithNoArgumentIsEveryEntryOldestFirst(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.sixNotes()

	got := noteTexts(f.h.notesAs("ann", "/tasks/"+f.issue+"/notes"))

	want := []string{"entry 1", "entry 2", "entry 3", "entry 4", "entry 5", "entry 6"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestLimitKeepsTheNewestEntriesStillOldestFirst(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.sixNotes()

	got := noteTexts(f.h.notesAs("ann", "/tasks/"+f.issue+"/notes?limit=2"))

	if strings.Join(got, ",") != "entry 5,entry 6" {
		t.Fatalf("got %v", got)
	}
}

func TestAfterAnEntryIdReturnsOnlyTheEntriesAddedSince(t *testing.T) {
	f := newMoreWritesFixture(t)
	ids := f.sixNotes()

	got := noteTexts(f.h.notesAs("ann", "/tasks/"+f.issue+"/notes?after="+ids[3]))

	if strings.Join(got, ",") != "entry 5,entry 6" {
		t.Fatalf("got %v", got)
	}
}

func TestAfterTheLastEntryReturnsAnEmptyList(t *testing.T) {
	f := newMoreWritesFixture(t)
	ids := f.sixNotes()

	got := f.h.notesAs("ann", "/tasks/"+f.issue+"/notes?after="+ids[5])

	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestAfterATimeReturnsOnlyStrictlyNewerEntries(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.sixNotes()
	all := f.h.notesAs("ann", "/tasks/"+f.issue+"/notes")
	third := all[2]["created_at"].(string)

	got := noteTexts(f.h.notesAs("ann", "/tasks/"+f.issue+"/notes?after="+third))

	if len(got) != 3 || got[0] != "entry 4" {
		t.Fatalf("got %v", got)
	}
}

func TestKindPersonReturnsOnlyPersonEntriesAndRepeatingKindsAddsMore(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.sixNotes()
	notes := "/tasks/" + f.issue + "/notes"

	people := noteTexts(f.h.notesAs("ann", notes+"?kind=person"))
	both := noteTexts(f.h.notesAs("ann", notes+"?kind=person&kind=worker"))
	newestPerson := noteTexts(f.h.notesAs("ann", notes+"?kind=person&limit=1"))

	if strings.Join(people, ",") != "entry 1,entry 3,entry 5" || len(both) != 6 || strings.Join(newestPerson, ",") != "entry 5" {
		t.Fatalf("people %v both %v newest %v", people, both, newestPerson)
	}
}

func TestABadAfterKindOrLimitIsRefusedWith400(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.sixNotes()
	notes := "/tasks/" + f.issue + "/notes"

	f.h.expect(http.StatusBadRequest, "ann", "GET", notes+"?after=no-such-note", nil)
	f.h.expect(http.StatusBadRequest, "ann", "GET", notes+"?kind=robot", nil)
	f.h.expect(http.StatusBadRequest, "ann", "GET", notes+"?limit=0", nil)
	f.h.expect(http.StatusBadRequest, "ann", "GET", notes+"?limit=201", nil)
	f.h.expect(http.StatusBadRequest, "ann", "GET", notes+"?limit=many", nil)
}
