package routes_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func newRuleNoteFixture(t *testing.T) *ruleFixture {
	t.Helper()
	f := newRuleFixture(t)
	for _, kind := range []string{"rule", "rule", "decision", "question"} {
		f.h.expect(http.StatusCreated, "bob", "POST", "/rules", map[string]any{"kind": kind, "title": "a " + kind})
	}
	return f
}

func (f *ruleFixture) noteOn(caller, code string, body map[string]any) map[string]any {
	f.h.t.Helper()
	return f.h.expect(http.StatusCreated, caller, "POST", "/rules/"+code+"/notes", body)
}

func TestACanSeeReaderAddsANoteAndTheHubSetsTheAuthorFromTheCaller(t *testing.T) {
	f := newRuleNoteFixture(t)

	note := f.noteOn("ann", "R-1", map[string]any{"text": "Seen in review.", "links": []string{"commit 3f9a1c2"}})

	if note["author_id"] != "ann" || note["kind"] != "note" || note["source"] != "person" || note["rule_code"] != "R-1" || note["created_at"] == "" {
		t.Fatalf("got %v", note)
	}
	if _, has := note["state"]; has {
		t.Fatalf("a plain note has no state: %v", note)
	}
}

func TestANoteGoesOnARuleADecisionAndAQuestionOverTheRoutes(t *testing.T) {
	f := newRuleNoteFixture(t)
	for _, code := range []string{"R-1", "D-1", "Q-1"} {
		if note := f.noteOn("ann", code, map[string]any{"text": "on " + code, "kind": "compatible", "related_codes": []string{"R-2"}}); note["rule_code"] != code {
			t.Fatalf("got %v", note)
		}
	}
	f.h.expect(http.StatusOK, "bob", "PUT", "/rules/R-1/status", map[string]any{"status": "in_force"})
	f.h.expect(http.StatusOK, "bob", "PUT", "/rules/R-1/status", map[string]any{"status": "retired"})
	f.noteOn("ann", "R-1", map[string]any{"text": "after it was retired"})
}

func TestARefusedNoteIsAnsweredWithTheRightStatusAndSavesNothing(t *testing.T) {
	f := newRuleNoteFixture(t)
	cases := []struct {
		name string
		want int
		code string
		body map[string]any
	}{
		{"wrong kind", http.StatusBadRequest, "R-1", map[string]any{"text": "x", "kind": "verdict"}},
		{"wrong source", http.StatusBadRequest, "R-1", map[string]any{"text": "x", "source": "oracle"}},
		{"empty text", http.StatusBadRequest, "R-1", map[string]any{"text": "  "}},
		{"text left out", http.StatusBadRequest, "R-1", map[string]any{"kind": "note"}},
		{"too long", http.StatusBadRequest, "R-1", map[string]any{"text": strings.Repeat("x", 4001)}},
		{"its own code", http.StatusBadRequest, "R-1", map[string]any{"text": "x", "related_codes": []string{"R-1"}}},
		{"unknown related code", http.StatusNotFound, "R-1", map[string]any{"text": "x", "related_codes": []string{"R-2", "R-99"}}},
		{"unknown record", http.StatusNotFound, "R-99", map[string]any{"text": "x"}},
	}

	for _, c := range cases {
		f.h.expect(c.want, "ann", "POST", "/rules/"+c.code+"/notes", c.body)
	}
	if got := f.list("ann", "/rules/notes"); len(got) != 0 {
		t.Fatalf("a refused note was saved: %v", got)
	}
}

func TestACallerWhoCannotSeeABacklogIsRefusedEveryNoteRoute(t *testing.T) {
	f := newRuleNoteFixture(t)
	note := f.noteOn("ann", "R-1", map[string]any{"kind": "contradiction", "text": "x", "related_codes": []string{"R-2"}})
	id := note["id"].(string)

	f.h.expect(http.StatusForbidden, "nog", "POST", "/rules/R-1/notes", map[string]any{"text": "x"})
	f.h.expect(http.StatusForbidden, "nog", "GET", "/rules/R-1/notes", nil)
	f.h.expect(http.StatusForbidden, "nog", "GET", "/rules/notes", nil)
	f.h.expect(http.StatusForbidden, "nog", "GET", "/rules/R-1", nil)
	f.h.expect(http.StatusForbidden, "nog", "PUT", "/rules/notes/"+id+"/resolve", map[string]any{})
}

func TestOnlyACanMoveCallerResolvesAContradictionNote(t *testing.T) {
	f := newRuleNoteFixture(t)
	id := f.noteOn("ann", "R-1", map[string]any{"kind": "contradiction", "text": "x", "related_codes": []string{"R-2"}})["id"].(string)

	f.h.expect(http.StatusForbidden, "ann", "PUT", "/rules/notes/"+id+"/resolve", map[string]any{})
	resolved := f.h.expect(http.StatusOK, "bob", "PUT", "/rules/notes/"+id+"/resolve", map[string]any{"resolution": "D-1 chose R-1.", "resolved_by_code": "D-1"})

	if resolved["state"] != "resolved" || resolved["resolved_by"] != "bob" || resolved["resolution"] != "D-1 chose R-1." || resolved["resolved_by_code"] != "D-1" || resolved["text"] != "x" || resolved["author_id"] != "ann" {
		t.Fatalf("got %v", resolved)
	}
	f.h.expect(http.StatusConflict, "bob", "PUT", "/rules/notes/"+id+"/resolve", map[string]any{})
}

func TestAResolveIsRefusedWithTheRightStatusForTheWrongNoteOrCode(t *testing.T) {
	f := newRuleNoteFixture(t)
	plain := f.noteOn("ann", "R-1", map[string]any{"text": "plain"})["id"].(string)
	clash := f.noteOn("ann", "R-1", map[string]any{"kind": "contradiction", "text": "clash", "related_codes": []string{"R-2"}})["id"].(string)

	f.h.expect(http.StatusBadRequest, "bob", "PUT", "/rules/notes/"+plain+"/resolve", map[string]any{})
	f.h.expect(http.StatusNotFound, "bob", "PUT", "/rules/notes/no-such-note/resolve", map[string]any{})
	f.h.expect(http.StatusNotFound, "bob", "PUT", "/rules/notes/"+clash+"/resolve", map[string]any{"resolved_by_code": "D-99"})
	f.h.expect(http.StatusBadRequest, "bob", "PUT", "/rules/notes/"+clash+"/resolve", map[string]any{"resolved_by_code": "Q-1"})
	if open := f.list("ann", "/rules/notes?kind=contradiction&state=open"); len(open) != 1 || open[0]["id"] != clash {
		t.Fatalf("open contradictions %v", open)
	}
}

func TestEveryOpenContradictionIsOneCallAndRelatedFindsEitherRecord(t *testing.T) {
	f := newRuleNoteFixture(t)
	firstAt := f.noteOn("ann", "R-1", map[string]any{"text": "plain"})["created_at"].(string)
	clash := f.noteOn("ann", "R-1", map[string]any{"kind": "contradiction", "text": "r1 against r2", "related_codes": []string{"R-2"}})
	first := clash["id"].(string)
	f.noteOn("ann", "D-1", map[string]any{"kind": "contradiction", "text": "d1 against r2", "source": "judge", "related_codes": []string{"R-2"}})
	f.h.expect(http.StatusOK, "bob", "PUT", "/rules/notes/"+first+"/resolve", map[string]any{})

	open := f.list("ann", "/rules/notes?kind=contradiction&state=open")
	if len(open) != 1 || open[0]["text"] != "d1 against r2" || open[0]["source"] != "judge" || open[0]["author_id"] != "ann" {
		t.Fatalf("open %v", open)
	}
	if got := f.list("ann", "/rules/notes?related=R-2"); len(got) != 2 {
		t.Fatalf("related R-2 %v", got)
	}
	if got := f.list("ann", "/rules/notes?related=R-1"); len(got) != 2 || got[1]["text"] != "r1 against r2" {
		t.Fatalf("related R-1 is the notes on it %v", got)
	}
	if got := f.list("ann", "/rules/notes?related=D-1"); len(got) != 1 || got[0]["text"] != "d1 against r2" {
		t.Fatalf("related D-1 %v", got)
	}
	if got := f.list("ann", "/rules/notes?kind=note&kind=contradiction"); len(got) != 3 {
		t.Fatalf("two kinds %v", got)
	}
	if got := f.list("ann", "/rules/R-1/notes"); len(got) != 2 || got[0]["text"] != "plain" {
		t.Fatalf("notes of R-1 %v", got)
	}
	if got := f.list("ann", "/rules/R-1/notes?kind=contradiction"); len(got) != 1 {
		t.Fatalf("contradictions of R-1 %v", got)
	}
	if got := f.list("ann", "/rules/notes?since="+url.QueryEscape(firstAt)); len(got) != 2 || got[0]["id"] != first {
		t.Fatalf("since the plain note %v", got)
	}
}

func TestABadNoteFilterIs400AndAnUnknownRecordIs404OnBothListRoutes(t *testing.T) {
	f := newRuleNoteFixture(t)
	for _, query := range []string{"kind=verdict", "state=pending", "since=yesterday", "limit=0", "limit=201"} {
		f.h.expect(http.StatusBadRequest, "ann", "GET", "/rules/notes?"+query, nil)
	}
	f.h.expect(http.StatusNotFound, "ann", "GET", "/rules/notes?rule=R-99", nil)
	f.h.expect(http.StatusNotFound, "ann", "GET", "/rules/R-99/notes", nil)
	f.h.expect(http.StatusBadRequest, "ann", "GET", "/rules/R-1/notes?kind=verdict", nil)
}

func TestReadingARuleReturnsItsNotesWithTheirStateAndTheCount(t *testing.T) {
	f := newRuleNoteFixture(t)
	f.noteOn("ann", "R-1", map[string]any{"text": "plain"})
	f.noteOn("ann", "R-1", map[string]any{"kind": "contradiction", "text": "clash", "related_codes": []string{"R-2"}})
	f.noteOn("ann", "R-2", map[string]any{"text": "elsewhere"})

	read := f.h.expect(http.StatusOK, "ann", "GET", "/rules/R-1", nil)

	notes, _ := read["notes"].([]any)
	if read["title"] != "a rule" || read["note_count"] != float64(2) || len(notes) != 2 {
		t.Fatalf("got %v", read)
	}
	if second := notes[1].(map[string]any); second["text"] != "clash" || second["state"] != "open" {
		t.Fatalf("got %v", second)
	}
	quiet := f.h.expect(http.StatusOK, "ann", "GET", "/rules/D-1", nil)
	if list, ok := quiet["notes"].([]any); !ok || len(list) != 0 || quiet["note_count"] != float64(0) {
		t.Fatalf("got %v", quiet)
	}
}

func TestTheNoteToolsExistAndTheOverlapAndCompatibleToolsStillDo(t *testing.T) {
	h := newAccessHub(t, map[string][]string{})
	for _, name := range []string{"taskmanager_rule_note_add", "taskmanager_rule_note_list", "taskmanager_rule_note_resolve"} {
		if h.tool(name).Description == "" {
			t.Fatalf("tool %s has no description", name)
		}
	}
	for _, name := range []string{"taskmanager_rule_overlap_list", "taskmanager_rule_compatible_add", "taskmanager_rule_compatible_remove"} {
		if h.tool(name).Description == "" {
			t.Fatalf("tool %s has no description", name)
		}
	}
	for _, tool := range h.tools {
		if tool.Name == "taskmanager_rule_note_list_of" {
			t.Fatalf("tool %s should not be published", tool.Name)
		}
	}
}

func TestAnAgentAddsAJudgeNoteAndResolvesItThroughTheTools(t *testing.T) {
	h := newAccessHub(t, map[string][]string{})
	for _, title := range []string{"A", "B"} {
		if _, err := h.invoke("agent", "taskmanager_rule_create", map[string]any{"kind": "rule", "title": title}); err != nil {
			t.Fatal(err)
		}
	}

	note, err := h.invoke("agent", "taskmanager_rule_note_add", map[string]any{"ruleCode": "R-1", "kind": "contradiction", "source": "judge", "text": "clash", "related_codes": []string{"R-2"}})
	if err != nil || note["state"] != "open" || note["source"] != "judge" || note["author_id"] != "agent" {
		t.Fatalf("got %v, %v", note, err)
	}
	done, err := h.invoke("agent", "taskmanager_rule_note_resolve", map[string]any{"noteID": note["id"], "resolution": "fine"})
	if err != nil || done["state"] != "resolved" || done["resolved_by"] != "agent" {
		t.Fatalf("got %v, %v", done, err)
	}
}

func TestAnUnknownFieldOnAddingANoteIs400AndSavesNothing(t *testing.T) {
	f := newRuleNoteFixture(t)
	bodies := []map[string]any{
		{"text": "x", "bogus": 1},
		{"kind": "contradiction", "text": "x", "relatedCodes": []string{"R-2"}},
		{"kind": "contradiction", "text": "x", "related_code": []string{"R-2"}},
		{"text": "x", "author_id": "someone-else"},
		{"text": "x", "RequesterID": "someone-else"},
		{"text": "x", "RuleCode": "R-2"},
	}

	for _, body := range bodies {
		f.h.expect(http.StatusBadRequest, "ann", "POST", "/rules/R-1/notes", body)
	}

	if got := f.list("ann", "/rules/notes"); len(got) != 0 {
		t.Fatalf("a refused note was saved: %v", got)
	}
}

func TestAnUnknownFieldOnResolvingANoteIs400AndTheNoteStaysOpen(t *testing.T) {
	f := newRuleNoteFixture(t)
	id := f.noteOn("ann", "R-1", map[string]any{"kind": "contradiction", "text": "x", "related_codes": []string{"R-2"}})["id"].(string)
	bodies := []map[string]any{
		{"bogus": 1},
		{"resolvedByCode": "D-1"},
		{"resolved_by": "someone-else"},
		{"NoteID": "other"},
		{"RequesterID": "someone-else"},
	}

	for _, body := range bodies {
		f.h.expect(http.StatusBadRequest, "bob", "PUT", "/rules/notes/"+id+"/resolve", body)
	}

	if open := f.list("ann", "/rules/notes?kind=contradiction&state=open"); len(open) != 1 {
		t.Fatalf("open contradictions %v", open)
	}
}

func TestTheNoteAddToolAsksForTheTextAndNamesTheAllowedKindsAndSources(t *testing.T) {
	h := newAccessHub(t, map[string][]string{})
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}

	if err := json.Unmarshal(h.tool("taskmanager_rule_note_add").InputSchema, &schema); err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(schema.Required, "text") || !slices.Contains(schema.Required, "ruleCode") {
		t.Fatalf("required %v", schema.Required)
	}
	if !slices.Equal(schema.Properties["kind"].Enum, []string{"note", "contradiction", "compatible", "question"}) || !slices.Equal(schema.Properties["source"].Enum, []string{"person", "worker", "judge"}) {
		t.Fatalf("properties %v", schema.Properties)
	}
}
