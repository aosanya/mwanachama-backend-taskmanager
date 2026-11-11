package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func text(s string) *string { return &s }

func newTaggedIssue(t *testing.T, mgr mwanachamataskmanager.TaskManager) mwanachamataskmanager.Task {
	t.Helper()
	return mustCreate(t, mgr, mwanachamataskmanager.Task{
		Title:       "HM1",
		Description: "old description",
		Priority:    mwanachamataskmanager.TaskPriorityCritical,
		Tags:        []string{"hungarian-municipality", "Ready"},
		ProjectName: "projects/hungarian-municipality",
		Rank:        777,
	})
}

func TestPatchTask_OnlyTheDescriptionSentLeavesTheRestAlone(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made, err := mgr.GetTask(ctx, newTaggedIssue(t, mgr).ID)
	if err != nil {
		t.Fatal(err)
	}

	patched, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made.ID, Description: text("new description")})
	if err != nil {
		t.Fatal(err)
	}
	read, err := mgr.GetTask(ctx, made.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, got := range []mwanachamataskmanager.Task{patched, read} {
		if got.Description != "new description" {
			t.Fatalf("description %q", got.Description)
		}
		if got.Title != "HM1" || got.Priority != mwanachamataskmanager.TaskPriorityCritical || got.ProjectName != "projects/hungarian-municipality" || got.Rank != 777 {
			t.Fatalf("a field not sent changed: %+v", got)
		}
		if len(got.Tags) != 2 {
			t.Fatalf("tags %v, want %v", got.Tags, made.Tags)
		}
		if got.Status != made.Status || got.Backlog != made.Backlog || got.Step != made.Step {
			t.Fatalf("status or place changed: %+v", got)
		}
	}
}

func TestPatchTask_EachFieldSentIsTheOnlyOneChanged(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := newTaggedIssue(t, mgr)
	priority := mwanachamataskmanager.TaskPriorityLow

	got, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made.ID, Title: text("HM1 renamed"), Priority: &priority, Tags: &[]string{"Blocked"}, ProjectName: text("projects/other")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "HM1 renamed" || got.Priority != priority || got.ProjectName != "projects/other" || !reflect.DeepEqual(got.Tags, []string{"Blocked"}) {
		t.Fatalf("got %+v", got)
	}
	if got.Description != "old description" || got.Rank != 777 {
		t.Fatalf("a field not sent changed: %+v", got)
	}
}

func TestPatchTask_AnEmptyTagListSentClearsTheTagsAndNothingElse(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := newTaggedIssue(t, mgr)

	got, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made.ID, Tags: &[]string{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tags) != 0 || got.Description != "old description" || got.Priority != mwanachamataskmanager.TaskPriorityCritical {
		t.Fatalf("got %+v", got)
	}
}

func TestPatchTask_AnUnknownIssueIsNotFound(t *testing.T) {
	mgr := newTestManager(t)

	_, err := mgr.PatchTaskAs(context.Background(), "", mwanachamataskmanager.TaskPatch{ID: "nope", Description: text("x")})
	if !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPatchTask_AStatusSentMovesTheLifecycleAndKeepsTheRest(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := newTaggedIssue(t, mgr)
	started := mwanachamataskmanager.TaskStatusInProgress

	got, err := mgr.PatchTaskAs(ctx, "", mwanachamataskmanager.TaskPatch{ID: made.ID, Status: &started})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != started || got.Rank != 777 || len(got.Tags) != 2 || got.Description != "old description" {
		t.Fatalf("got %+v", got)
	}
}
