package mwanachamataskmanager_test

import (
	"context"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func TestClockStart_TheHubsOwnDependencyNotesNeverStartTheClock(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	waiting := issueWithTags(t, mgr, "waiting", "Waiting")
	dependency := issueWithTags(t, mgr, "dependency", "Ready")
	linkDependsOn(t, mgr, waiting, dependency)
	finishAt(t, mgr, dependency.ID, "done")
	if got := logTexts(t, mgr, waiting.ID); len(got) != 1 {
		t.Fatalf("the hub should have logged its dependency note, log = %v", got)
	}

	if got := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{waiting.ID}})[waiting.ID].StartedAt; got != "" {
		t.Fatalf("started_at %q before any worker or person wrote", got)
	}

	first, err := mgr.AddTaskNote(ctx, "worker", waiting.ID, "worker", "taken", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := nearestOf(t, mgr, mwanachamataskmanager.CapsNearestFilter{TaskIDs: []string{waiting.ID}})[waiting.ID].StartedAt; got != first.CreatedAt {
		t.Fatalf("started_at %q, want the first worker entry %q", got, first.CreatedAt)
	}
}
