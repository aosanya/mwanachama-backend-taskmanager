package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func archivedOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) bool {
	t.Helper()
	list, err := mgr.ListBacklogs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range list.Items {
		if item.ID == id {
			return item.Archived
		}
	}
	t.Fatalf("backlog %q is not listed", id)
	return false
}

func TestBacklogArchive_AnArchivedBacklogIsMarkedAndComesBack(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateBacklog(ctx, "Redesign", []string{"Open", "Archive"}); err != nil {
		t.Fatal(err)
	}
	if archivedOf(t, mgr, "redesign") {
		t.Fatal("a new backlog starts archived")
	}
	archived, err := mgr.ArchiveBacklog(ctx, "redesign")
	if err != nil || !archived.Archived {
		t.Fatalf("archive = %+v, %v", archived, err)
	}
	if !archivedOf(t, mgr, "redesign") || archivedOf(t, mgr, "backlog") {
		t.Fatal("the list does not mark exactly the archived backlog")
	}
	back, err := mgr.UnarchiveBacklog(ctx, "redesign")
	if err != nil || back.Archived {
		t.Fatalf("unarchive = %+v, %v", back, err)
	}
	if archivedOf(t, mgr, "redesign") {
		t.Fatal("an unarchived backlog is still marked")
	}
}

func TestBacklogArchive_IssuesStayAndStillOpen(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.CreateBacklog(ctx, "Redesign", []string{"Open", "Archive"}); err != nil {
		t.Fatal(err)
	}
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "a"})
	if _, err := mgr.MoveTaskToStep(ctx, task.ID, "redesign", "archive"); err != nil {
		t.Fatal(err)
	}
	summary, err := mgr.ArchiveBacklog(ctx, "redesign")
	if err != nil || summary.Issues != 1 {
		t.Fatalf("archive = %+v, %v", summary, err)
	}
	got, err := mgr.GetTask(ctx, task.ID)
	if err != nil || got.Backlog != "redesign" || got.Step != "archive" || got.Code != task.Code {
		t.Fatalf("issue after archive = %+v, %v", got, err)
	}
}

func TestBacklogArchive_TheFirstBacklogAndUnknownOnesAreRefused(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	if _, err := mgr.ArchiveBacklog(ctx, "backlog"); err == nil {
		t.Fatal("the first backlog was archived")
	}
	if _, err := mgr.ArchiveBacklog(ctx, "nowhere"); err == nil {
		t.Fatal("an unknown backlog was archived")
	}
	if archivedOf(t, mgr, "backlog") {
		t.Fatal("the refused archive left a mark")
	}
}

func TestBacklogArchive_NeedsCanEditOnTheBacklog(t *testing.T) {
	ctx := context.Background()
	mgr := restrictedManager(t)
	if _, err := giveAccess(mgr, ctx, "software", devGroup, "move"); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.ArchiveBacklogAs(ctx, "dev", "software")
	wantForbidden(t, "archive", err)
	_, err = mgr.UnarchiveBacklogAs(ctx, "dev", "software")
	wantForbidden(t, "unarchive", err)
	if archivedOf(t, mgr, "software") {
		t.Fatal("a refused archive left a mark")
	}
	if _, err := giveAccess(mgr, ctx, "software", devGroup, "edit"); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.ArchiveBacklogAs(ctx, "dev", "software")
	if err != nil || !got.Archived {
		t.Fatalf("archive with Can edit = %+v, %v", got, err)
	}
}
