package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func noteDraft(who, code, kind, source, text string, related, links []string) mwanachamataskmanager.RuleNoteDraft {
	return mwanachamataskmanager.RuleNoteDraft{RequesterID: who, RuleCode: code, Kind: kind, Source: source, Text: text, RelatedCodes: related, Links: links}
}

func noteResolution(who, noteID, resolution, byCode string) mwanachamataskmanager.RuleNoteResolution {
	return mwanachamataskmanager.RuleNoteResolution{RequesterID: who, NoteID: noteID, Resolution: resolution, ResolvedByCode: byCode}
}

func addNote(t *testing.T, mgr mwanachamataskmanager.TaskManager, code, kind, text string, related ...string) mwanachamataskmanager.RuleNote {
	t.Helper()
	note, err := mgr.AddRuleNote(context.Background(), noteDraft("ann", code, kind, "", text, related, nil))
	if err != nil {
		t.Fatalf("note on %s: %v", code, err)
	}
	return note
}

func notesOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, filter mwanachamataskmanager.RuleNoteFilter) []mwanachamataskmanager.RuleNote {
	t.Helper()
	filter.RequesterID = "ann"
	notes, err := mgr.ListRuleNotes(context.Background(), filter)
	if err != nil {
		t.Fatalf("list %+v: %v", filter, err)
	}
	return notes
}

func noteTexts(notes []mwanachamataskmanager.RuleNote) string {
	var texts []string
	for _, note := range notes {
		texts = append(texts, note.Text)
	}
	return strings.Join(texts, ",")
}

func TestRuleNotes_ANoteTakesItsAuthorFromTheCallerAndDefaultsToAPersonsPlainNote(t *testing.T) {
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "No comments")

	note, err := mgr.AddRuleNote(context.Background(), noteDraft("ann", rule.Code, "", "", "  Seen in review.  ", nil, []string{" commit 3f9a1c2 ", ""}))
	if err != nil {
		t.Fatal(err)
	}

	if note.AuthorID != "ann" || note.Kind != "note" || note.Source != "person" || note.State != "" {
		t.Fatalf("got %+v", note)
	}
	if note.ID == "" || note.RuleID != rule.ID || note.RuleCode != "R-1" || note.CreatedAt == "" {
		t.Fatalf("got %+v", note)
	}
	if note.Text != "Seen in review." || len(note.Links) != 1 || note.Links[0] != "commit 3f9a1c2" || note.RelatedCodes == nil {
		t.Fatalf("got %+v", note)
	}
}

func TestRuleNotes_AContradictionStartsOpenAndEveryOtherKindHasNoState(t *testing.T) {
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "A")
	mustCreateRule(t, mgr, "rule", "B")

	for kind, want := range map[string]string{"contradiction": "open", "compatible": "", "question": "", "note": ""} {
		if got := addNote(t, mgr, rule.Code, kind, "x "+kind, "R-2"); got.State != want || got.Kind != kind {
			t.Fatalf("%s: got %+v, want state %q", kind, got, want)
		}
	}
}

func TestRuleNotes_ASourceIsPersonWorkerOrJudgeAndTheAuthorStaysTheCaller(t *testing.T) {
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "A")

	for _, source := range []string{"person", "worker", "judge"} {
		note, err := mgr.AddRuleNote(context.Background(), noteDraft("robot", rule.Code, "note", source, "by "+source, nil, nil))
		if err != nil || note.Source != source || note.AuthorID != "robot" {
			t.Fatalf("%s: got %+v, err %v", source, note, err)
		}
	}
	if _, err := mgr.AddRuleNote(context.Background(), noteDraft("robot", rule.Code, "note", "oracle", "x", nil, nil)); !errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
		t.Fatalf("err %v, want ErrInvalidRuleNote", err)
	}
}

func TestRuleNotes_AWrongKindAnEmptyTextOrATooLongTextIsRefusedAndNothingIsSaved(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "A")
	cases := map[string][2]string{
		"unknown kind": {"verdict", "x"},
		"empty text":   {"note", ""},
		"blank text":   {"note", " \n\t "},
		"too long":     {"note", strings.Repeat("x", 4001)},
	}

	for name, c := range cases {
		if _, err := mgr.AddRuleNote(ctx, noteDraft("ann", rule.Code, c[0], "", c[1], nil, nil)); !errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
			t.Fatalf("%s: err %v, want ErrInvalidRuleNote", name, err)
		}
	}
	if _, err := mgr.AddRuleNote(ctx, noteDraft("ann", rule.Code, "note", "", strings.Repeat("x", 4000), nil, nil)); err != nil {
		t.Fatalf("a text at the maximum: %v", err)
	}
	if got := notesOf(t, mgr, mwanachamataskmanager.RuleNoteFilter{Rule: rule.Code}); len(got) != 1 {
		t.Fatalf("saved %d notes, want only the one at the maximum", len(got))
	}
}

func TestRuleNotes_AnUnknownRelatedCodeIsNotFoundAndTheNoteItsOwnCodeOrTooManyCodesIsInvalid(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "A")
	mustCreateRule(t, mgr, "rule", "B")

	_, unknown := mgr.AddRuleNote(ctx, noteDraft("ann", rule.Code, "contradiction", "", "x", []string{"R-2", "R-99"}, nil))
	if !errors.Is(unknown, mwanachamataskmanager.ErrRuleNoteCodeNotFound) || errors.Is(unknown, mwanachamataskmanager.ErrInvalidRule) || errors.Is(unknown, mwanachamataskmanager.ErrInvalidRuleNote) {
		t.Fatalf("unknown related: err %v, want only ErrRuleNoteCodeNotFound", unknown)
	}
	_, self := mgr.AddRuleNote(ctx, noteDraft("ann", rule.Code, "contradiction", "", "x", []string{"R-99", "R-1"}, nil))
	if !errors.Is(self, mwanachamataskmanager.ErrInvalidRuleNote) || errors.Is(self, mwanachamataskmanager.ErrRuleNoteCodeNotFound) {
		t.Fatalf("own code: err %v, want only ErrInvalidRuleNote", self)
	}
	many := make([]string, 21)
	for i := range many {
		many[i] = fmt.Sprintf("R-%d", i+2)
	}
	if _, err := mgr.AddRuleNote(ctx, noteDraft("ann", rule.Code, "note", "", "x", many, nil)); !errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
		t.Fatalf("too many codes: err %v, want ErrInvalidRuleNote", err)
	}
	if got := notesOf(t, mgr, mwanachamataskmanager.RuleNoteFilter{}); len(got) != 0 {
		t.Fatalf("a refused note was saved: %+v", got)
	}
}

func TestRuleNotes_ARelatedCodeIsKeptOnceAndTheNoteOnAnUnknownRecordIsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "A")
	mustCreateRule(t, mgr, "decision", "B")

	note := addNote(t, mgr, rule.Code, "compatible", "fit", " D-1 ", "D-1", "")
	if len(note.RelatedCodes) != 1 || note.RelatedCodes[0] != "D-1" {
		t.Fatalf("got %+v", note.RelatedCodes)
	}
	if _, err := mgr.AddRuleNote(ctx, noteDraft("ann", "R-77", "note", "", "x", nil, nil)); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("err %v, want ErrRuleNotFound", err)
	}
}

func TestRuleNotes_ANoteGoesOnARuleADecisionAndAQuestionInAnyStatus(t *testing.T) {
	mgr := newTestManager(t)
	rule := inForceRule(t, mgr, "old rule")
	next, err := mgr.SupersedeRule(context.Background(), "owner", rule.Code, mwanachamataskmanager.RuleRecord{Title: "new rule"})
	if err != nil {
		t.Fatal(err)
	}
	decision := mustCreateRule(t, mgr, "decision", "choice")
	question := mustCreateRule(t, mgr, "question", "open matter")
	retired := mustCreateRule(t, mgr, "rule", "to retire")
	mustSetStatus(t, mgr, retired.Code, "retired")

	for _, code := range []string{rule.Code, next.Code, decision.Code, question.Code, retired.Code} {
		note := addNote(t, mgr, code, "note", "on "+code)
		if note.RuleCode != code {
			t.Fatalf("got %+v", note)
		}
	}
	if got := mustReadRule(t, mgr, rule.Code); got.Status != "superseded" {
		t.Fatalf("a note changed the record: %+v", got)
	}
}

func TestRuleNotes_ANoteLandsOnTheEndOfItsRecordsLogOldestFirst(t *testing.T) {
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "A")
	other := mustCreateRule(t, mgr, "rule", "B")

	for _, text := range []string{"one", "two", "three", "four"} {
		addNote(t, mgr, rule.Code, "note", text)
		addNote(t, mgr, other.Code, "note", "other "+text)
	}

	if got := noteTexts(notesOf(t, mgr, mwanachamataskmanager.RuleNoteFilter{Rule: rule.Code})); got != "one,two,three,four" {
		t.Fatalf("got %s", got)
	}
	if got := len(notesOf(t, mgr, mwanachamataskmanager.RuleNoteFilter{})); got != 8 {
		t.Fatalf("every record's notes: got %d, want 8", got)
	}
}

func TestRuleNotes_AListNarrowsByKindStateRecordAndTime(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	rule := mustCreateRule(t, mgr, "rule", "A")
	mustCreateRule(t, mgr, "rule", "B")
	first := addNote(t, mgr, rule.Code, "note", "plain")
	clash := addNote(t, mgr, rule.Code, "contradiction", "clash", "R-2")
	addNote(t, mgr, rule.Code, "contradiction", "second clash", "R-2")
	if _, err := mgr.ResolveRuleNote(ctx, noteResolution("owner", clash.ID, "", "")); err != nil {
		t.Fatal(err)
	}
	addNote(t, mgr, "R-2", "compatible", "fit", "R-1")

	by := func(f mwanachamataskmanager.RuleNoteFilter) string { return noteTexts(notesOf(t, mgr, f)) }
	if got := by(mwanachamataskmanager.RuleNoteFilter{Kind: []string{"contradiction"}}); got != "clash,second clash" {
		t.Fatalf("by kind: %s", got)
	}
	if got := by(mwanachamataskmanager.RuleNoteFilter{Kind: []string{"contradiction"}, State: "open"}); got != "second clash" {
		t.Fatalf("open contradictions: %s", got)
	}
	if got := by(mwanachamataskmanager.RuleNoteFilter{State: "resolved"}); got != "clash" {
		t.Fatalf("resolved: %s", got)
	}
	if got := by(mwanachamataskmanager.RuleNoteFilter{Kind: []string{"note", "compatible"}}); got != "plain,fit" {
		t.Fatalf("by two kinds: %s", got)
	}
	if got := by(mwanachamataskmanager.RuleNoteFilter{Since: first.CreatedAt}); got != "clash,second clash,fit" {
		t.Fatalf("since: %s", got)
	}
	if got := by(mwanachamataskmanager.RuleNoteFilter{Limit: 2}); got != "second clash,fit" {
		t.Fatalf("limit keeps the newest: %s", got)
	}
}

func TestRuleNotes_AListByRelatedCodeFindsNotesWhereTheCodeIsOnEitherRecord(t *testing.T) {
	mgr := newTestManager(t)
	mustCreateRule(t, mgr, "rule", "A")
	mustCreateRule(t, mgr, "rule", "B")
	mustCreateRule(t, mgr, "decision", "C")
	addNote(t, mgr, "R-1", "contradiction", "r1 names r2", "R-2")
	addNote(t, mgr, "R-2", "note", "r2 alone")
	addNote(t, mgr, "D-1", "compatible", "d1 names r2", "R-2", "R-1")

	related := func(code string) string {
		return noteTexts(notesOf(t, mgr, mwanachamataskmanager.RuleNoteFilter{Related: code}))
	}
	if got := related("R-2"); got != "r1 names r2,r2 alone,d1 names r2" {
		t.Fatalf("R-2: %s", got)
	}
	if got := related("R-1"); got != "r1 names r2,d1 names r2" {
		t.Fatalf("R-1: %s", got)
	}
	if got := related("D-1"); got != "d1 names r2" {
		t.Fatalf("D-1: %s", got)
	}
	if got := related("R-99"); got != "" {
		t.Fatalf("an unknown code in a filter finds nothing, got %s", got)
	}
}

func TestRuleNotes_ABadFilterIsInvalidAndANoteListOfAnUnknownRecordIsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	mustCreateRule(t, mgr, "rule", "A")
	bad := map[string]mwanachamataskmanager.RuleNoteFilter{
		"kind":  {Kind: []string{"verdict"}},
		"state": {State: "pending"},
		"since": {Since: "yesterday"},
		"low":   {Limit: -1},
		"high":  {Limit: 201},
	}

	for name, filter := range bad {
		filter.RequesterID = "ann"
		_, err := mgr.ListRuleNotes(ctx, filter)
		if !errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNoteFilter) || errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
			t.Fatalf("%s: err %v, want only ErrInvalidRuleNoteFilter", name, err)
		}
	}
	if _, err := mgr.ListRuleNotes(ctx, mwanachamataskmanager.RuleNoteFilter{RequesterID: "ann", Rule: "R-99"}); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("err %v, want ErrRuleNotFound", err)
	}
}
