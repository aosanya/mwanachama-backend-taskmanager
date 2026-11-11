package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestTaskNotes_AnIssueStartsWithNoNotes(t *testing.T) {
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	notes, err := mgr.ListTaskNotes(context.Background(), "", task.ID)
	if err != nil || len(notes) != 0 {
		t.Fatalf("got %d notes, err %v, want none", len(notes), err)
	}
}

func TestTaskNotes_AddingKeepsWriterKindTextLinksAndTime(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	note, err := mgr.AddTaskNote(ctx, "worker-1", task.ID, "worker", "Fixed the importer.", []string{"https://example.test/commit/3f9a1c2", "loader/importer.go"})
	if err != nil {
		t.Fatal(err)
	}
	if note.ID == "" || note.TaskID != task.ID || note.AuthorID != "worker-1" || note.Kind != "worker" || note.Text != "Fixed the importer." || note.CreatedAt == "" {
		t.Fatalf("got %+v", note)
	}
	notes, err := mgr.ListTaskNotes(ctx, "worker-1", task.ID)
	if err != nil || len(notes) != 1 {
		t.Fatalf("got %d notes, err %v", len(notes), err)
	}
	if got := notes[0].Links; len(got) != 2 || got[0] != "https://example.test/commit/3f9a1c2" || got[1] != "loader/importer.go" {
		t.Fatalf("links %v", got)
	}
}

func TestTaskNotes_ListIsOldestFirstAndNothingIsOverwritten(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	other := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "b"})

	for _, text := range []string{"first", "second", "third"} {
		if _, err := mgr.AddTaskNote(ctx, "owner", task.ID, "person", text, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mgr.AddTaskNote(ctx, "owner", other.ID, "person", "elsewhere", nil); err != nil {
		t.Fatal(err)
	}
	notes, err := mgr.ListTaskNotes(ctx, "owner", task.ID)
	if err != nil || len(notes) != 3 {
		t.Fatalf("got %d notes, err %v", len(notes), err)
	}
	for i, want := range []string{"first", "second", "third"} {
		if notes[i].Text != want {
			t.Fatalf("note %d is %q, want %q", i, notes[i].Text, want)
		}
	}
}

func TestTaskNotes_ANoteNeedsTextAnIssueAndAKnownKind(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	if _, err := mgr.AddTaskNote(ctx, "owner", task.ID, "person", "   ", nil); !errors.Is(err, mwanachamataskmanager.ErrInvalidTaskNote) {
		t.Fatalf("blank text: %v", err)
	}
	if _, err := mgr.AddTaskNote(ctx, "owner", task.ID, "robot", "x", nil); !errors.Is(err, mwanachamataskmanager.ErrInvalidTaskNote) {
		t.Fatalf("unknown kind: %v", err)
	}
	if _, err := mgr.AddTaskNote(ctx, "owner", "no-such-issue", "person", "x", nil); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("unknown issue: %v", err)
	}
	if _, err := mgr.ListTaskNotes(ctx, "owner", "no-such-issue"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("list unknown issue: %v", err)
	}
}

func TestTaskNotes_AKindLeftOutIsAPerson(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})

	note, err := mgr.AddTaskNote(ctx, "owner", task.ID, "", "Checked it.", nil)
	if err != nil || note.Kind != "person" {
		t.Fatalf("got %+v, err %v", note, err)
	}
}

func TestTaskNotes_ACanSeeReaderMayAddButAnUnseenIssueIsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	task := softwareTask(t, mgr)
	if _, err := mgr.GiveBacklogAccess(ctx, "software", devGroup, "see"); err != nil {
		t.Fatal(err)
	}

	if _, err := mgr.MoveTaskToStepAs(ctx, "dev", task.ID, "software", "shipped"); !errors.Is(err, mwanachamataskmanager.ErrBacklogForbidden) {
		t.Fatalf("a Can see reader moves: %v", err)
	}
	if _, err := mgr.AddTaskNote(ctx, "dev", task.ID, "person", "Reviewed on a phone.", nil); err != nil {
		t.Fatalf("a Can see reader adds a note: %v", err)
	}
	notes, err := mgr.ListTaskNotes(ctx, "dev", task.ID)
	if err != nil || len(notes) != 1 || notes[0].AuthorID != "dev" {
		t.Fatalf("got %+v, err %v", notes, err)
	}
	if _, err := mgr.AddTaskNote(ctx, "none", task.ID, "person", "x", nil); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("a person who cannot see the issue adds: %v", err)
	}
	if _, err := mgr.ListTaskNotes(ctx, "none", task.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("a person who cannot see the issue lists: %v", err)
	}
}
