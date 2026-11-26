package mwanachamataskmanager_test

import (
	"context"
	"reflect"
	"testing"
)

func TestReadRule_CarriesTheCompatibleCodesAndTheNotesTogether(t *testing.T) {
	ctx := context.Background()
	mgr, a, b := overlappingPair(t)
	if err := mgr.AddRuleCompatible(ctx, "owner", a.Code, b.Code); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddRuleNote(ctx, noteDraft("ann", a.Code, "note", "person", "Seen in review.", nil, nil)); err != nil {
		t.Fatal(err)
	}

	read, err := mgr.ReadRule(ctx, "owner", a.Code)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(read.Compatible, []string{b.Code}) {
		t.Fatalf("compatible %v, want [%s]", read.Compatible, b.Code)
	}
	if read.NoteCount != 1 || len(read.Notes) != 1 || read.Notes[0].Text != "Seen in review." {
		t.Fatalf("notes %+v count %d", read.Notes, read.NoteCount)
	}
}

func TestReadRule_ARuleWithNeitherReadsEmptyListsAndNotNull(t *testing.T) {
	ctx := context.Background()
	mgr, a, _ := overlappingPair(t)

	read, err := mgr.ReadRule(ctx, "owner", a.Code)
	if err != nil {
		t.Fatal(err)
	}
	if read.Compatible == nil || read.Notes == nil || read.NoteCount != 0 {
		t.Fatalf("compatible %v notes %v count %d", read.Compatible, read.Notes, read.NoteCount)
	}
}
