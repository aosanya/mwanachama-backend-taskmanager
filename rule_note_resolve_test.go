package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func resolveNote(mgr mwanachamataskmanager.TaskManager, id, resolution, byCode string) (mwanachamataskmanager.RuleNote, error) {
	return mgr.ResolveRuleNote(context.Background(), noteResolution("boss", id, resolution, byCode))
}

func noteWithID(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) mwanachamataskmanager.RuleNote {
	t.Helper()
	for _, note := range notesOf(t, mgr, mwanachamataskmanager.RuleNoteFilter{}) {
		if note.ID == id {
			return note
		}
	}
	t.Fatalf("note %s not listed", id)
	return mwanachamataskmanager.RuleNote{}
}

func twoRulesAndADecision(t *testing.T) mwanachamataskmanager.TaskManager {
	t.Helper()
	mgr := newTestManager(t)
	mustCreateRule(t, mgr, "rule", "A")
	mustCreateRule(t, mgr, "rule", "B")
	mustCreateRule(t, mgr, "decision", "C")
	mustCreateRule(t, mgr, "question", "D")
	return mgr
}

func TestRuleNoteResolve_ResolvingSetsTheStateTheCallerAndTheTimeAndChangesNothingElse(t *testing.T) {
	mgr := twoRulesAndADecision(t)
	before := addNote(t, mgr, "R-1", "contradiction", "R-1 and R-2 disagree", "R-2")

	got, err := resolveNote(mgr, before.ID, "  D-1 chose R-1.  ", "D-1")
	if err != nil {
		t.Fatal(err)
	}

	if got.State != "resolved" || got.ResolvedBy != "boss" || got.ResolvedAt == "" || got.Resolution != "D-1 chose R-1." || got.ResolvedByCode != "D-1" {
		t.Fatalf("got %+v", got)
	}
	stored := noteWithID(t, mgr, before.ID)
	if stored.State != "resolved" || stored.ResolvedBy != "boss" || stored.ResolvedByCode != "D-1" {
		t.Fatalf("stored %+v", stored)
	}
	stored.State, stored.ResolvedAt, stored.ResolvedBy, stored.Resolution, stored.ResolvedByCode = "open", "", "", "", ""
	if fmt.Sprintf("%+v", stored) != fmt.Sprintf("%+v", before) {
		t.Fatalf("resolving changed more than the resolution:\n after  %+v\n before %+v", stored, before)
	}
}

func TestRuleNoteResolve_TheResolutionAndTheSettlingCodeAreOptionalAndARuleCanSettleIt(t *testing.T) {
	mgr := twoRulesAndADecision(t)
	bare := addNote(t, mgr, "R-1", "contradiction", "one", "R-2")
	byRule := addNote(t, mgr, "R-1", "contradiction", "two", "R-2")

	if got, err := resolveNote(mgr, bare.ID, "", ""); err != nil || got.State != "resolved" || got.Resolution != "" || got.ResolvedByCode != "" {
		t.Fatalf("got %+v, err %v", got, err)
	}
	if got, err := resolveNote(mgr, byRule.ID, "", " R-2 "); err != nil || got.ResolvedByCode != "R-2" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestRuleNoteResolve_ResolvingTwiceIsAConflictAndTheFirstResolutionStands(t *testing.T) {
	mgr := twoRulesAndADecision(t)
	note := addNote(t, mgr, "R-1", "contradiction", "x", "R-2")
	if _, err := resolveNote(mgr, note.ID, "first", ""); err != nil {
		t.Fatal(err)
	}

	_, err := mgr.ResolveRuleNote(context.Background(), noteResolution("other", note.ID, "second", "D-1"))

	if !errors.Is(err, mwanachamataskmanager.ErrRuleNoteResolved) || errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
		t.Fatalf("err %v, want only ErrRuleNoteResolved", err)
	}
	if got := noteWithID(t, mgr, note.ID); got.Resolution != "first" || got.ResolvedBy != "boss" || got.ResolvedByCode != "" {
		t.Fatalf("the second resolve changed it: %+v", got)
	}
}

func TestRuleNoteResolve_OnlyAContradictionCanBeResolved(t *testing.T) {
	mgr := twoRulesAndADecision(t)
	for _, kind := range []string{"note", "compatible", "question"} {
		note := addNote(t, mgr, "R-1", kind, "a "+kind)
		_, err := resolveNote(mgr, note.ID, "", "")
		if !errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) || errors.Is(err, mwanachamataskmanager.ErrRuleNoteResolved) {
			t.Fatalf("%s: err %v, want only ErrInvalidRuleNote", kind, err)
		}
		if got := noteWithID(t, mgr, note.ID); got.State != "" || got.ResolvedBy != "" {
			t.Fatalf("%s: %+v", kind, got)
		}
	}
}

func TestRuleNoteResolve_AnUnknownNoteOrSettlingCodeIsNotFoundAndAQuestionCannotSettleIt(t *testing.T) {
	mgr := twoRulesAndADecision(t)
	note := addNote(t, mgr, "R-1", "contradiction", "x", "R-2")

	if _, err := resolveNote(mgr, "no-such-note", "", ""); !errors.Is(err, mwanachamataskmanager.ErrRuleNoteNotFound) {
		t.Fatalf("unknown note: err %v, want ErrRuleNoteNotFound", err)
	}
	if _, err := resolveNote(mgr, note.ID, "", "D-99"); !errors.Is(err, mwanachamataskmanager.ErrRuleNoteCodeNotFound) || errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
		t.Fatalf("unknown settling code: err %v, want only ErrRuleNoteCodeNotFound", err)
	}
	if _, err := resolveNote(mgr, note.ID, "", "Q-1"); !errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
		t.Fatalf("a question settling it: err %v, want ErrInvalidRuleNote", err)
	}
	if _, err := resolveNote(mgr, note.ID, strings.Repeat("x", 4001), ""); !errors.Is(err, mwanachamataskmanager.ErrInvalidRuleNote) {
		t.Fatalf("too long a resolution: err %v, want ErrInvalidRuleNote", err)
	}
	if got := noteWithID(t, mgr, note.ID); got.State != "open" {
		t.Fatalf("a refused resolve changed it: %+v", got)
	}
}

func TestRuleNoteRead_ReadingARuleReturnsTheLatestFiftyNotesOldestFirstWithTheCount(t *testing.T) {
	mgr := newTestManager(t)
	mustCreateRule(t, mgr, "rule", "A")
	mustCreateRule(t, mgr, "rule", "B")
	for i := 1; i <= 55; i++ {
		addNote(t, mgr, "R-1", "note", fmt.Sprintf("n%02d", i))
	}
	addNote(t, mgr, "R-2", "note", "elsewhere")
	addNote(t, mgr, "R-2", "contradiction", "names R-1", "R-1")

	view, err := mgr.ReadRule(context.Background(), "ann", "R-1")
	if err != nil {
		t.Fatal(err)
	}

	if view.Code != "R-1" || view.Title != "A" || view.NoteCount != 55 || len(view.Notes) != 50 {
		t.Fatalf("code %s count %d notes %d", view.Code, view.NoteCount, len(view.Notes))
	}
	if view.Notes[0].Text != "n06" || view.Notes[49].Text != "n55" {
		t.Fatalf("got %s .. %s, want n06 .. n55", view.Notes[0].Text, view.Notes[49].Text)
	}
}

func TestRuleNoteRead_ARuleWithNoNotesReadsAnEmptyListAndAnUnknownOneIsNotFound(t *testing.T) {
	mgr := newTestManager(t)
	mustCreateRule(t, mgr, "rule", "A")

	view, err := mgr.ReadRule(context.Background(), "ann", "R-1")
	if err != nil || view.Notes == nil || len(view.Notes) != 0 || view.NoteCount != 0 {
		t.Fatalf("got %+v, err %v", view, err)
	}
	if _, err := mgr.ReadRule(context.Background(), "ann", "R-9"); !errors.Is(err, mwanachamataskmanager.ErrRuleNotFound) {
		t.Fatalf("err %v, want ErrRuleNotFound", err)
	}
}

func TestRuleNoteSentinels_EachStandsAloneSoNoMappingOrderCanAnswerTheWrongStatus(t *testing.T) {
	stands := []error{
		mwanachamataskmanager.ErrInvalidRuleNote, mwanachamataskmanager.ErrInvalidRuleNoteFilter,
		mwanachamataskmanager.ErrRuleNoteCodeNotFound, mwanachamataskmanager.ErrRuleNoteNotFound, mwanachamataskmanager.ErrRuleNoteResolved,
	}
	others := append([]error{mwanachamataskmanager.ErrInvalidRule, mwanachamataskmanager.ErrRuleNotFound, mwanachamataskmanager.ErrRuleChanged}, stands...)
	for _, sentinel := range stands {
		for _, other := range others {
			if sentinel != other && errors.Is(sentinel, other) {
				t.Fatalf("%v wraps %v", sentinel, other)
			}
		}
	}
}
