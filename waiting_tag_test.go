package mwanachamataskmanager_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func issueWithTags(t *testing.T, mgr mwanachamataskmanager.TaskManager, title string, tags ...string) mwanachamataskmanager.Task {
	t.Helper()
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: title, Tags: tags})
	if _, err := mgr.MoveTaskToStep(context.Background(), task.ID, "backlog", "to-do"); err != nil {
		t.Fatalf("MoveTaskToStep to-do: %v", err)
	}
	return task
}

func tagsOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) []string {
	t.Helper()
	task, err := mgr.GetTask(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return task.Tags
}

func wantTags(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string, want ...string) {
	t.Helper()
	got := slices.Clone(tagsOf(t, mgr, id))
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("tags = %v, want %v", got, want)
	}
}

func finishAt(t *testing.T, mgr mwanachamataskmanager.TaskManager, id, step string) {
	t.Helper()
	if _, err := mgr.MoveTaskToStep(context.Background(), id, "backlog", step); err != nil {
		t.Fatalf("MoveTaskToStep %s: %v", step, err)
	}
}

func logTexts(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) []string {
	t.Helper()
	notes, err := mgr.ListTaskNotes(context.Background(), "", id)
	if err != nil {
		t.Fatalf("ListTaskNotes: %v", err)
	}
	var texts []string
	for _, n := range notes {
		texts = append(texts, n.Kind+": "+n.Text)
	}
	return texts
}

func TestWaitingTag_FinishingTheLastOpenDependencyDropsWaitingAndAddsReady(t *testing.T) {
	mgr := newTestManager(t)
	a := issueWithTags(t, mgr, "A", "Waiting", "hub-steward-swe")
	b := issueWithTags(t, mgr, "B", "Ready")
	c := issueWithTags(t, mgr, "C", "Ready")
	linkDependsOn(t, mgr, a, b)
	linkDependsOn(t, mgr, a, c)

	finishAt(t, mgr, b.ID, "done")
	wantTags(t, mgr, a.ID, "Waiting", "hub-steward-swe")
	if got := logTexts(t, mgr, a.ID); len(got) != 0 {
		t.Fatalf("log after the first dependency = %v, want empty", got)
	}

	finishAt(t, mgr, c.ID, "done")
	wantTags(t, mgr, a.ID, "Ready", "hub-steward-swe")
	got := logTexts(t, mgr, a.ID)
	finished, err := mgr.GetTask(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Code == "" {
		t.Fatal("the finished issue has no code to name")
	}
	if len(got) != 1 || !strings.HasPrefix(got[0], "worker: ") || !strings.Contains(got[0], finished.Code) {
		t.Fatalf("log = %v, want one worker entry naming the issue that finished", got)
	}
}

func TestWaitingTag_CancellingTheLastOpenDependencyDropsWaitingToo(t *testing.T) {
	mgr := newTestManager(t)
	a := issueWithTags(t, mgr, "A", "Waiting")
	b := issueWithTags(t, mgr, "B", "Ready")
	c := issueWithTags(t, mgr, "C", "Ready")
	linkDependsOn(t, mgr, a, b)
	linkDependsOn(t, mgr, a, c)

	finishAt(t, mgr, b.ID, "cancelled")
	wantTags(t, mgr, a.ID, "Waiting")
	finishAt(t, mgr, c.ID, "done")
	wantTags(t, mgr, a.ID, "Ready")
	if got := logTexts(t, mgr, a.ID); len(got) != 1 {
		t.Fatalf("log = %v, want one entry", got)
	}
}

func TestWaitingTag_ABlockedIssueIsNeverTouched(t *testing.T) {
	mgr := newTestManager(t)
	a := issueWithTags(t, mgr, "A", "Blocked")
	b := issueWithTags(t, mgr, "B", "Ready")
	linkDependsOn(t, mgr, a, b)
	wantTags(t, mgr, a.ID, "Blocked")

	finishAt(t, mgr, b.ID, "done")
	wantTags(t, mgr, a.ID, "Blocked")
	if got := logTexts(t, mgr, a.ID); len(got) != 0 {
		t.Fatalf("log = %v, want empty", got)
	}
}

func TestWaitingTag_ANewEdgeToAnOpenIssueAddsWaitingAndRemovesReady(t *testing.T) {
	mgr := newTestManager(t)
	a := issueWithTags(t, mgr, "A", "Ready", "hub-steward-swe")
	b := issueWithTags(t, mgr, "B", "Ready")
	linkDependsOn(t, mgr, a, b)
	wantTags(t, mgr, a.ID, "Waiting", "hub-steward-swe")
}

func TestWaitingTag_ANewEdgeToAFinishedIssueLeavesTheTagsAlone(t *testing.T) {
	mgr := newTestManager(t)
	a := issueWithTags(t, mgr, "A", "Ready")
	b := issueWithTags(t, mgr, "B", "Ready")
	finishAt(t, mgr, b.ID, "done")
	linkDependsOn(t, mgr, a, b)
	wantTags(t, mgr, a.ID, "Ready")
}

func TestWaitingTag_RemovingTheLastOpenEdgeDropsWaitingAndAddsReady(t *testing.T) {
	mgr := newTestManager(t)
	a := issueWithTags(t, mgr, "A", "Ready")
	b := issueWithTags(t, mgr, "B", "Ready")
	c := issueWithTags(t, mgr, "C", "Ready")
	linkDependsOn(t, mgr, a, b)
	linkDependsOn(t, mgr, a, c)
	ctx := context.Background()

	if err := mgr.DeleteRelationship(ctx, a.ID, b.ID, mwanachamataskmanager.RelLabelDependsOn); err != nil {
		t.Fatal(err)
	}
	wantTags(t, mgr, a.ID, "Waiting")
	if err := mgr.DeleteRelationship(ctx, a.ID, c.ID, mwanachamataskmanager.RelLabelDependsOn); err != nil {
		t.Fatal(err)
	}
	wantTags(t, mgr, a.ID, "Ready")
}
