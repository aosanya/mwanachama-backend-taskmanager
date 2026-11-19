package mwanachamataskmanager_test

import (
	"context"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"gorm.io/gorm"
	"testing"
	"time"
)

const noteMomentForm = "2006-01-02T15:04:05.000000000Z07:00"

func noteTable(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var table string
	if err := db.Raw(`SELECT m.name FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.type = 'table' AND p.name = 'automatic' AND m.name NOT LIKE '%\_test' ESCAPE '\'`).Scan(&table).Error; err != nil || table == "" {
		t.Fatalf("no task_note table: %v", err)
	}
	return table
}
func moveNotesBack(t *testing.T, mgr mwanachamataskmanager.TaskManager, db *gorm.DB, taskID string, by time.Duration) {
	t.Helper()
	table := noteTable(t, db)
	for _, note := range logAll(t, mgr, taskID) {
		at, err := time.Parse(noteMomentForm, note.CreatedAt)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("UPDATE "+table+" SET created_at = ? WHERE id = ?", at.Add(-by).UTC().Format(noteMomentForm), note.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
}
func inToDoWithAWorkerNote(t *testing.T, mgr mwanachamataskmanager.TaskManager, db *gorm.DB) mwanachamataskmanager.Task {
	t.Helper()
	issue := issueWithTags(t, mgr, "collected notes", "Ready")
	if _, err := mgr.AddTaskNote(context.Background(), "", issue.ID, "worker", "a plan written while waiting", nil); err != nil {
		t.Fatal(err)
	}
	moveNotesBack(t, mgr, db, issue.ID, 24*time.Hour)
	return issue
}
func claimedNoteTime(t *testing.T, mgr mwanachamataskmanager.TaskManager, taskID string) string {
	t.Helper()
	claim, err := mgr.ClaimTaskAs(context.Background(), "", mwanachamataskmanager.ClaimRequest{TaskID: taskID})
	if err != nil {
		t.Fatal(err)
	}
	notes, err := mgr.ListTaskNotes(context.Background(), "", taskID)
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range notes {
		if note.ID == claim.NoteID {
			return note.CreatedAt
		}
	}
	t.Fatalf("the Taken entry %s is not in the Log", claim.NoteID)
	return ""
}
func wantStartedAtTheClaim(t *testing.T, mgr mwanachamataskmanager.TaskManager, taskID, claimedAt string) {
	t.Helper()
	item := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{taskID}})[taskID]
	if item.StartedAt != claimedAt {
		t.Fatalf("started_at %q, want the Taken entry %q", item.StartedAt, claimedAt)
	}
	started, err := time.Parse(noteMomentForm, item.StartedAt)
	if err != nil {
		t.Fatal(err)
	}
	if age := time.Since(started); age < 0 || age > time.Minute {
		t.Fatalf("elapsed %v, want about the claim age", age)
	}
	if item.Interrupted || (item.Nearest != nil && item.Nearest.Ratio >= 1) {
		t.Fatalf("a freshly taken issue is stopped or timed up: %+v", item)
	}
}
func TestClockStartsAtTheClaim_AWorkerNoteFromADayBeforeTheMoveDoesNotStartIt(t *testing.T) {
	mgr, db, _ := newTestManagerWithDB(t)
	issue := inToDoWithAWorkerNote(t, mgr, db)
	claimedAt := claimedNoteTime(t, mgr, issue.ID)
	wantStartedAtTheClaim(t, mgr, issue.ID, claimedAt)
}
func TestClockStartsAtTheClaim_AnIssueResumedAfterAReleaseStartsAgainAtItsNewTakenEntry(t *testing.T) {
	mgr, db, _ := newTestManagerWithDB(t)
	issue := inToDoWithAWorkerNote(t, mgr, db)
	claimedNoteTime(t, mgr, issue.ID)
	moveNotesBack(t, mgr, db, issue.ID, 2*time.Hour)
	finishAt(t, mgr, issue.ID, "to-do")
	if _, err := mgr.AddTaskNote(context.Background(), "", issue.ID, "worker", "released with the work half done", nil); err != nil {
		t.Fatal(err)
	}
	moveNotesBack(t, mgr, db, issue.ID, 30*time.Minute)
	resumedAt := claimedNoteTime(t, mgr, issue.ID)
	wantStartedAtTheClaim(t, mgr, issue.ID, resumedAt)
}
func TestClockStartsAtTheClaim_AnIssueNeverMovedKeepsTheEarliestWorkerNote(t *testing.T) {
	mgr, db, _ := newTestManagerWithDB(t)
	issue := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "never moved"})
	if _, err := mgr.AddTaskNote(context.Background(), "", issue.ID, "worker", "first", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AddTaskNote(context.Background(), "", issue.ID, "worker", "second", nil); err != nil {
		t.Fatal(err)
	}
	moveNotesBack(t, mgr, db, issue.ID, 3*time.Hour)
	item := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{issue.ID}})[issue.ID]
	first := logAll(t, mgr, issue.ID)[0]
	if item.StartedAt != first.CreatedAt {
		t.Fatalf("started_at %q, want the earliest worker note %q", item.StartedAt, first.CreatedAt)
	}
}
func logAll(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) []mwanachamataskmanager.TaskNote {
	t.Helper()
	notes, err := mgr.ListTaskNotes(context.Background(), "", id)
	if err != nil {
		t.Fatal(err)
	}
	return notes
}
