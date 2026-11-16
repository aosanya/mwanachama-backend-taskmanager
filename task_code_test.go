package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

func codeOf(t *testing.T, mgr mwanachamataskmanager.TaskManager, task mwanachamataskmanager.Task) mwanachamataskmanager.Task {
	t.Helper()
	return mustCreate(t, mgr, task)
}

func TestTaskCode_ANewIssueTakesTheNextNumberOfItsRepoSeries(t *testing.T) {
	mgr := newTestManager(t)
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "first", ProjectName: "hub-steward-swe", Code: "SW191"})
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "other repo", ProjectName: "kazi", Code: "DEV-5"})

	second := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "second", ProjectName: "hub-steward-swe"})
	third := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "third", ProjectName: "hub-steward-swe"})
	other := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "kazi next", ProjectName: "kazi"})

	if second.Code != "SW192" || third.Code != "SW193" || other.Code != "DEV-6" {
		t.Fatalf("got %q, %q and %q, want SW192, SW193 and DEV-6", second.Code, third.Code, other.Code)
	}
	if third.CodeSeries != "SW" || third.CodeNumber != 193 {
		t.Fatalf("got series %q number %d, want SW and 193", third.CodeSeries, third.CodeNumber)
	}
}

func TestTaskCode_ARepoWithNoCodedIssueStartsTheDefaultSeries(t *testing.T) {
	mgr := newTestManager(t)

	first := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "first", ProjectName: "brand-new-repo"})
	second := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "second", ProjectName: "brand-new-repo"})

	if first.Code != "ISS-1" || second.Code != "ISS-2" {
		t.Fatalf("got %q and %q, want ISS-1 and ISS-2", first.Code, second.Code)
	}
}

func TestTaskCode_TheRepoSeriesIsTheOneMostOfItsIssuesCarry(t *testing.T) {
	mgr := newTestManager(t)
	for _, code := range []string{"DEV-1", "DEV-2", "DEV-3", "W9"} {
		codeOf(t, mgr, mwanachamataskmanager.Task{Title: code, ProjectName: "mixed", Code: code})
	}

	next := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "next", ProjectName: "mixed"})

	if next.Code != "DEV-4" {
		t.Fatalf("got %q, want DEV-4", next.Code)
	}
}

func TestTaskCode_ACodeGivenAtCreateIsKeptAndNumberingContinuesPastIt(t *testing.T) {
	mgr := newTestManager(t)
	kept := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "loaded row", ProjectName: "kazi", Code: "DEV-127"})
	next := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "raised later", ProjectName: "kazi"})

	if kept.Code != "DEV-127" || next.Code != "DEV-128" {
		t.Fatalf("got %q and %q, want DEV-127 and DEV-128", kept.Code, next.Code)
	}
}

func TestTaskCode_TwoIssuesOfOneRepoCannotShareACode(t *testing.T) {
	mgr := newTestManager(t)
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "a", ProjectName: "kazi", Code: "DEV-1"})

	_, err := mgr.CreateTask(context.Background(), mwanachamataskmanager.Task{Title: "b", ProjectName: "kazi", Code: "DEV-1"})

	if !errors.Is(err, mwanachamataskmanager.ErrDuplicateTaskCode) {
		t.Fatalf("got %v, want ErrDuplicateTaskCode", err)
	}
}

func TestTaskCode_TheSameCodeMayStandInAnotherRepo(t *testing.T) {
	mgr := newTestManager(t)
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "a", ProjectName: "kazi", Code: "DEV-1"})

	other := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "b", ProjectName: "member", Code: "DEV-1"})

	if other.Code != "DEV-1" {
		t.Fatalf("got %q, want DEV-1", other.Code)
	}
}

func TestTaskCode_AnOverwriteSetsTheCodeAndAnUpdateWithoutOneKeepsIt(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "a", ProjectName: "kazi"})

	made.Code = "DEV-900"
	renamed, err := mgr.UpdateTask(ctx, made)
	if err != nil || renamed.Code != "DEV-900" || renamed.CodeSeries != "DEV-" || renamed.CodeNumber != 900 {
		t.Fatalf("overwrite gave %+v, %v", renamed, err)
	}

	renamed.Code = ""
	renamed.Title = "a, retitled"
	kept, err := mgr.UpdateTask(ctx, renamed)
	if err != nil || kept.Code != "DEV-900" {
		t.Fatalf("update without a code gave %q, %v, want DEV-900", kept.Code, err)
	}
	read, err := mgr.GetTask(ctx, made.ID)
	if err != nil || read.Code != "DEV-900" || read.CodeNumber != 900 {
		t.Fatalf("read back %+v, %v", read, err)
	}
}

func TestTaskCode_AnOverwriteToACodeAnotherIssueHoldsIsRefused(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "a", ProjectName: "kazi", Code: "DEV-1"})
	second := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "b", ProjectName: "kazi", Code: "DEV-2"})

	second.Code = "DEV-1"
	_, err := mgr.UpdateTask(ctx, second)

	if !errors.Is(err, mwanachamataskmanager.ErrDuplicateTaskCode) {
		t.Fatalf("got %v, want ErrDuplicateTaskCode", err)
	}
	read, _ := mgr.GetTask(ctx, second.ID)
	if read.Code != "DEV-2" {
		t.Fatalf("code is now %q, want DEV-2 unchanged", read.Code)
	}
}

func TestTaskCode_AnIssueKeepsItsOwnCodeWhenItIsSentBackUnchanged(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "a", ProjectName: "kazi", Code: "DEV-1"})

	made.Description = "more"
	again, err := mgr.UpdateTask(ctx, made)

	if err != nil || again.Code != "DEV-1" {
		t.Fatalf("got %q, %v, want DEV-1", again.Code, err)
	}
}

func TestTaskCode_ADeletedIssueNumberIsNotGivenOutAgain(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "a", ProjectName: "kazi", Code: "DEV-7"})
	gone := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "b", ProjectName: "kazi"})
	if gone.Code != "DEV-8" {
		t.Fatalf("got %q, want DEV-8", gone.Code)
	}
	if err := mgr.DeleteTask(ctx, gone.ID); err != nil {
		t.Fatal(err)
	}

	next := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "c", ProjectName: "kazi"})

	if next.Code != "DEV-9" {
		t.Fatalf("got %q, want DEV-9", next.Code)
	}
}

func TestTaskCode_ACodeOutsideTheUsualShapeIsKeptAsWrittenAndNumberingGoesOn(t *testing.T) {
	mgr := newTestManager(t)
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "a", ProjectName: "swe", Code: "SW190"})
	odd := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "b", ProjectName: "swe", Code: "SW29a"})
	next := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "c", ProjectName: "swe"})

	if odd.Code != "SW29a" || odd.CodeSeries != "" || odd.CodeNumber != 0 {
		t.Fatalf("got %+v, want the code as written with no series or number", odd)
	}
	if next.Code != "SW191" {
		t.Fatalf("got %q, want SW191", next.Code)
	}
}

func TestTaskCode_TheCodeIsOnEveryReadAndListing(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	made := codeOf(t, mgr, mwanachamataskmanager.Task{Title: "a", ProjectName: "kazi", Code: "DEV-1"})

	listed, err := mgr.ListTasks(ctx, mwanachamataskmanager.TaskFilter{})
	if err != nil || len(listed) != 1 || listed[0].Code != "DEV-1" {
		t.Fatalf("listed %+v, %v", listed, err)
	}
	pending := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{})
	if len(pending.Items) != 1 || pending.Items[0].Code != "DEV-1" || pending.Items[0].ID != made.ID {
		t.Fatalf("pending %+v", pending.Items)
	}
}

func TestTaskCode_SearchFindsAnIssueByItsCodeEvenWhenTheTitleDoesNotCarryIt(t *testing.T) {
	mgr := newTestManager(t)
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "Gives every new issue its number", ProjectName: "swe", Code: "SW193"})
	codeOf(t, mgr, mwanachamataskmanager.Task{Title: "Something else", ProjectName: "swe", Code: "SW194"})

	found := pendingOf(t, mgr, mwanachamataskmanager.PendingFilter{Q: "sw193"})

	if titles := itemTitles(found); len(titles) != 1 || titles[0] != "Gives every new issue its number" {
		t.Fatalf("got %v", titles)
	}
}
