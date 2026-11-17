package mwanachamataskmanager_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

const longLegacyTitle = "Write rpc_hand_over() and rpc_complete_handover() for the party plane"

const longLegacyName = "Write rpc_hand_over() and rpc_complete_h"

func blankCodes(t *testing.T, db *gorm.DB, table string, tasks ...mwanachamataskmanager.Task) {
	t.Helper()
	for _, task := range tasks {
		if err := db.Exec("update "+table+" set code = '', code_series = '', code_number = 0 where id = ?", task.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func blankTitle(t *testing.T, db *gorm.DB, table string, task mwanachamataskmanager.Task) {
	t.Helper()
	if err := db.Exec("update "+table+" set title = '' where id = ?", task.ID).Error; err != nil {
		t.Fatal(err)
	}
}

func TestDisplayName_UnblockImpactNamesWaitersWithoutACodeByTheirTrimmedTitle(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	root := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Root cause", Code: "R1"})
	short := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "  Short waiter  ", Code: "TMP1"})
	long := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: longLegacyTitle, Code: "TMP2"})
	coded := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Coded waiter", Code: "W3"})
	for _, waiter := range []mwanachamataskmanager.Task{short, long, coded} {
		linkDependsOn(t, mgr, waiter, root)
	}
	blankCodes(t, db, table, short, long)

	out, err := mgr.UnblockImpact(context.Background(), mwanachamataskmanager.UnblockImpactFilter{})
	if err != nil {
		t.Fatalf("UnblockImpact: %v", err)
	}

	if len(out.Items) != 1 || out.Items[0].Code != "R1" {
		t.Fatalf("items = %+v, want the one root with its own code", out.Items)
	}
	want := []string{"Short waiter", "W3", longLegacyName}
	slices.Sort(want)
	if !slices.Equal(out.Items[0].Dependents, want) {
		t.Fatalf("dependents = %q, want %q", out.Items[0].Dependents, want)
	}
}

func TestDisplayName_UnblockImpactKeepsAnEmptyCodeOnTheItemItself(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	root := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Legacy root", Code: "TMP0"})
	waiter := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Waits on legacy", Code: "W1"})
	linkDependsOn(t, mgr, waiter, root)
	blankCodes(t, db, table, root)

	out, err := mgr.UnblockImpact(context.Background(), mwanachamataskmanager.UnblockImpactFilter{})
	if err != nil {
		t.Fatalf("UnblockImpact: %v", err)
	}

	if len(out.Items) != 1 || out.Items[0].Code != "" || out.Items[0].Title != "Legacy root" {
		t.Fatalf("items = %+v, want the item's code field left as stored", out.Items)
	}
}

func TestDisplayName_ATitleLessAndCodeLessIssueIsNamedByItsId(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	root := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Root cause", Code: "R1"})
	bare := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "placeholder", Code: "TMP1"})
	linkDependsOn(t, mgr, bare, root)
	blankCodes(t, db, table, bare)
	blankTitle(t, db, table, bare)

	out, err := mgr.UnblockImpact(context.Background(), mwanachamataskmanager.UnblockImpactFilter{})
	if err != nil {
		t.Fatalf("UnblockImpact: %v", err)
	}

	if len(out.Items) != 1 || !slices.Equal(out.Items[0].Dependents, []string{bare.ID}) {
		t.Fatalf("items = %+v, want the dependent named by its id %q", out.Items, bare.ID)
	}
}

func TestDisplayName_BoardSnapshotNamesTheOldestIssueTheAgentLoadAndTheWaitsOnWithoutACode(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	working := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Working without a code", Code: "TMP1", AssignedTo: "agent-1"})
	working.Status = mwanachamataskmanager.TaskStatusInProgress
	working, err := mgr.UpdateTask(context.Background(), working)
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	blocked := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Blocked with a code", Code: "B1", Tags: []string{"Blocked"}})
	cause := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: longLegacyTitle, Code: "TMP2"})
	linkDependsOn(t, mgr, blocked, cause)
	blankCodes(t, db, table, working, cause)

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	step, ok := stepOf(snap, "In Progress")
	if !ok || step.OldestCode != "Working without a code" {
		t.Fatalf("in progress step = %+v (found %v), want the oldest issue named by its title", step, ok)
	}
	if len(snap.Agents) != 1 || !slices.Equal(snap.Agents[0].Codes, []string{"Working without a code"}) {
		t.Fatalf("agents = %+v, want the agent's issue named by its title", snap.Agents)
	}
	if len(snap.BlockedChains) != 1 || snap.BlockedChains[0].Code != "B1" {
		t.Fatalf("blocked chains = %+v, want the blocked issue with its own code", snap.BlockedChains)
	}
	waits := snap.BlockedChains[0].WaitsOn
	if len(waits) != 1 || waits[0].Code != longLegacyName {
		t.Fatalf("waits on = %+v, want %q", waits, longLegacyName)
	}
}

func TestDisplayName_BoardSnapshotKeepsTheCodeFieldOfAStalledAndABlockedIssueAsStored(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	stalled := startIssue(t, mgr, mwanachamataskmanager.Task{Title: "Stalled legacy", Code: "TMP1"})
	blankCodes(t, db, table, stalled)
	if err := db.Exec("update "+table+" set updated_at = ? where id = ?", "2020-01-01T00:00:00Z", stalled.ID).Error; err != nil {
		t.Fatal(err)
	}

	snap := snapshotOf(t, mgr, mwanachamataskmanager.BoardSnapshotFilter{})

	if len(snap.Stalled) != 1 || snap.Stalled[0].Code != "" || snap.Stalled[0].Title != "Stalled legacy" {
		t.Fatalf("stalled = %+v, want the code field empty and the title present", snap.Stalled)
	}
}

func TestDisplayName_NextReadyBatchNamesTheClashingIssueByItsTrimmedTitleWhenItHasNoCode(t *testing.T) {
	mgr, db, table := newTestManagerWithDB(t)
	running := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: longLegacyTitle, Code: "TMP1", Files: []string{"zz/a/"}})
	if _, err := mgr.MoveTaskToStep(context.Background(), running.ID, "backlog", "doing"); err != nil {
		t.Fatalf("MoveTaskToStep: %v", err)
	}
	mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Ready and clashing", Code: "B1", Tags: []string{"Ready"}, Files: []string{"zz/a/x.go"}})
	blankCodes(t, db, table, running)

	out, err := mgr.NextReadyBatch(context.Background(), mwanachamataskmanager.NextReadyBatchFilter{Count: 3})
	if err != nil {
		t.Fatalf("NextReadyBatch: %v", err)
	}

	if len(out.Items) != 0 || len(out.Skipped) != 1 {
		t.Fatalf("batch = %+v, want the one ready issue skipped", out)
	}
	if got := out.Skipped[0]; got.Code != "B1" || got.ClashesWith != longLegacyName {
		t.Fatalf("skipped = %+v, want clashes_with %q", got, longLegacyName)
	}
}

func TestDisplayName_AnIssueWithACodeIsNamedByItsCodeEverywhere(t *testing.T) {
	mgr := newTestManager(t)
	root := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "Root cause", Code: "R1"})
	waiter := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: strings.Repeat("long title ", 8), Code: "W1"})
	linkDependsOn(t, mgr, waiter, root)

	out, err := mgr.UnblockImpact(context.Background(), mwanachamataskmanager.UnblockImpactFilter{})
	if err != nil {
		t.Fatalf("UnblockImpact: %v", err)
	}

	if len(out.Items) != 1 || !slices.Equal(out.Items[0].Dependents, []string{"W1"}) {
		t.Fatalf("items = %+v, want the dependent named by its code", out.Items)
	}
}
