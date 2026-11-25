package mwanachamataskmanager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

type runHub struct {
	mgr    mwanachamataskmanager.TaskManager
	run    mwanachamataskmanager.WorkflowRun
	shown  mwanachamataskmanager.Task
	hidden mwanachamataskmanager.Task
}

func runWithIssues(t *testing.T, devAccess string, withHidden bool) runHub {
	t.Helper()
	ctx := context.Background()
	mgr := restrictedManager(t)
	run, err := mgr.CreateWorkflowRun(ctx, "release", "manual", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if run, err = mgr.UpdateWorkflowRunStatus(ctx, run.ID, mwanachamataskmanager.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatal(err)
	}
	shown := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in software", Backlog: "software", Step: "open", WorkflowRunID: run.ID})
	var hidden mwanachamataskmanager.Task
	if withHidden {
		hidden = mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in the first backlog", WorkflowRunID: run.ID})
	}
	if devAccess != "" {
		if _, err := giveLevel(mgr, ctx, "software", devGroup, devAccess); err != nil {
			t.Fatal(err)
		}
	}
	return runHub{mgr: mgr, run: run, shown: shown, hidden: hidden}
}

func TestRunGuards_AllTheIssuesOfARunMustBeOnesThePersonCanMove(t *testing.T) {
	ctx := context.Background()
	hub := runWithIssues(t, "move", true)
	deadline := time.Now().Add(time.Minute)

	_, err := hub.mgr.CancelWorkflowRunAs(ctx, "dev", hub.run.ID, "stop", "dev", deadline)
	wantForbidden(t, "cancel a run that holds an issue in a backlog dev cannot move", err)
	_, err = hub.mgr.RollbackWorkflowRunAs(ctx, "dev", hub.run.ID, "undo")
	wantForbidden(t, "roll back a run that holds an issue in a backlog dev cannot move", err)

	got, err := hub.mgr.GetWorkflowRun(ctx, hub.run.ID)
	if err != nil || got.Status != mwanachamataskmanager.WorkflowRunStatusInProgress {
		t.Fatalf("a refused cancel must leave the run in progress: %+v %v", got, err)
	}
}

func TestRunGuards_APersonWithCanMoveOnEveryIssueCancelsTheRun(t *testing.T) {
	ctx := context.Background()
	hub := runWithIssues(t, "move", false)

	cancelled, err := hub.mgr.CancelWorkflowRunAs(ctx, "dev", hub.run.ID, "stop", "dev", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("cancel with Can move on every issue: %v", err)
	}
	if cancelled.Status != mwanachamataskmanager.WorkflowRunStatusCancelling || cancelled.CancelledBy != "dev" || cancelled.CancelReason != "stop" {
		t.Fatalf("the run should be cancelling, by dev, with the reason: %+v", cancelled)
	}
	again, err := hub.mgr.CancelWorkflowRunAs(ctx, "owner", hub.run.ID, "again", "owner", time.Now().Add(time.Minute))
	if err != nil || again.Status != mwanachamataskmanager.WorkflowRunStatusCancelling || again.CancelReason != "stop" {
		t.Fatalf("cancelling a run already cancelling changes nothing: %+v %v", again, err)
	}
}

func TestRunGuards_TheOwnerGroupIsNeverNarrowedOnARun(t *testing.T) {
	ctx := context.Background()
	hub := runWithIssues(t, "", true)

	cancelled, err := hub.mgr.CancelWorkflowRunAs(ctx, "owner", hub.run.ID, "stop", "owner", time.Now().Add(time.Minute))
	if err != nil || cancelled.Status != mwanachamataskmanager.WorkflowRunStatusCancelling {
		t.Fatalf("the owner cancels any run: %+v %v", cancelled, err)
	}
	if _, err := hub.mgr.CancelWorkflowRunAs(ctx, "owner", "missing-run", "stop", "owner", time.Now().Add(time.Minute)); err == nil {
		t.Fatal("cancelling a run that does not exist should be refused")
	}
}

func TestRunGuards_ReadingARunsIssuesHidesWhatThePersonCannotSee(t *testing.T) {
	ctx := context.Background()
	hub := runWithIssues(t, "see", true)

	forDev, err := hub.mgr.ListTasksForRunFor(ctx, "dev", hub.run.ID)
	if err != nil || len(forDev) != 1 || forDev[0].ID != hub.shown.ID {
		t.Fatalf("dev sees %+v %v, want only the software issue", forDev, err)
	}
	forOwner, err := hub.mgr.ListTasksForRunFor(ctx, "owner", hub.run.ID)
	if err != nil || len(forOwner) != 2 {
		t.Fatalf("owner sees %d issues of the run, want 2: %v", len(forOwner), err)
	}
	all, err := hub.mgr.ListTasksForRun(ctx, hub.run.ID)
	if err != nil || len(all) != 2 {
		t.Fatalf("the unfiltered read lists both: %d %v", len(all), err)
	}
}

type caseHub struct {
	mgr    mwanachamataskmanager.TaskManager
	c      mwanachamataskmanager.Case
	shown  mwanachamataskmanager.Task
	hidden mwanachamataskmanager.Task
}

func caseWithTwoBacklogsOfIssues(t *testing.T, devAccess string) caseHub {
	t.Helper()
	ctx := context.Background()
	mgr := restrictedManager(t)
	c, err := mgr.OpenCase(ctx, "amina", mwanachamataskmanager.Case{SubjectKind: "market", SubjectID: "offer-1"})
	if err != nil {
		t.Fatal(err)
	}
	shown := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in software", Backlog: "software", Step: "open"})
	hidden := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "in the first backlog"})
	for _, id := range []string{shown.ID, hidden.ID} {
		if err := mgr.LinkCaseTask(ctx, c.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	if devAccess != "" {
		if _, err := giveLevel(mgr, ctx, "software", devGroup, devAccess); err != nil {
			t.Fatal(err)
		}
	}
	return caseHub{mgr: mgr, c: c, shown: shown, hidden: hidden}
}

func TestCaseGuards_CancellingACaseNeedsCanMoveOnEveryIssueItHolds(t *testing.T) {
	ctx := context.Background()
	hub := caseWithTwoBacklogsOfIssues(t, "move")

	_, err := hub.mgr.EndCaseAs(ctx, "dev", hub.c.ID, mwanachamataskmanager.CaseStatusCancelled, "no")
	wantForbidden(t, "cancel a case holding an issue dev cannot move", err)
	still, err := hub.mgr.GetCase(ctx, hub.c.ID)
	if err != nil || still.Status != mwanachamataskmanager.CaseStatusOpen {
		t.Fatalf("a refused cancel leaves the case open: %+v %v", still, err)
	}

	ended, err := hub.mgr.EndCaseAs(ctx, "owner", hub.c.ID, mwanachamataskmanager.CaseStatusCancelled, "no longer needed")
	if err != nil || ended.Status != mwanachamataskmanager.CaseStatusCancelled || ended.Reply != "no longer needed" {
		t.Fatalf("the owner cancels the case: %+v %v", ended, err)
	}
	for _, id := range []string{hub.shown.ID, hub.hidden.ID} {
		task, err := hub.mgr.GetTask(ctx, id)
		if err != nil || task.Status != mwanachamataskmanager.TaskStatusCancelled {
			t.Fatalf("cancelling a case cancels its open issues: %+v %v", task, err)
		}
	}
}

func TestCaseGuards_EndingACaseDoneIsNotHeldToCanMoveButNeedsItsIssuesSettled(t *testing.T) {
	ctx := context.Background()
	hub := caseWithTwoBacklogsOfIssues(t, "see")

	if _, err := hub.mgr.EndCaseAs(ctx, "dev", hub.c.ID, mwanachamataskmanager.CaseStatusDone, "done"); !errors.Is(err, mwanachamataskmanager.ErrCaseHasOpenTask) {
		t.Fatalf("ending a case done with open issues: got %v, want ErrCaseHasOpenTask", err)
	}
	if _, err := hub.mgr.EndCase(ctx, hub.c.ID, mwanachamataskmanager.CaseStatus("paused"), ""); !errors.Is(err, mwanachamataskmanager.ErrInvalidCase) {
		t.Fatalf("a case ends done or cancelled only: got %v", err)
	}
	if _, err := hub.mgr.EndCase(ctx, "missing", mwanachamataskmanager.CaseStatusDone, ""); !errors.Is(err, mwanachamataskmanager.ErrCaseNotFound) {
		t.Fatalf("ending a missing case: got %v", err)
	}
}

func TestCaseReads_ACasesIssuesAreHiddenWhereThePersonCannotSeeTheirBacklog(t *testing.T) {
	ctx := context.Background()
	hub := caseWithTwoBacklogsOfIssues(t, "see")

	forDev, err := hub.mgr.ListCaseTasksFor(ctx, "dev", hub.c.ID)
	if err != nil || len(forDev) != 1 || forDev[0].ID != hub.shown.ID {
		t.Fatalf("dev sees %+v %v, want only the software issue", forDev, err)
	}
	forOwner, err := hub.mgr.ListCaseTasksFor(ctx, "owner", hub.c.ID)
	if err != nil || len(forOwner) != 2 {
		t.Fatalf("owner sees %d, want 2: %v", len(forOwner), err)
	}
	if _, err := hub.mgr.ListCaseTasksFor(ctx, "dev", "missing"); !errors.Is(err, mwanachamataskmanager.ErrCaseNotFound) {
		t.Fatalf("a missing case: got %v", err)
	}
}

func TestCases_ListingMyCasesNeedsACallerAndShowsOnlyTheirOwn(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	mine := openCaseBy(t, mgr, "amina")
	openCaseBy(t, mgr, "otieno")

	if _, err := mgr.ListMyCases(ctx, "", mwanachamataskmanager.CaseFilter{}); !errors.Is(err, mwanachamataskmanager.ErrInvalidCase) {
		t.Fatalf("no caller: got %v, want ErrInvalidCase", err)
	}
	got, err := mgr.ListMyCases(ctx, "amina", mwanachamataskmanager.CaseFilter{RequesterID: "otieno"})
	if err != nil || len(got) != 1 || got[0].ID != mine.ID {
		t.Fatalf("asking for someone else's cases still gives the caller's own: %+v %v", got, err)
	}
	if _, err := mgr.OpenCase(ctx, "", mwanachamataskmanager.Case{}); !errors.Is(err, mwanachamataskmanager.ErrInvalidCase) {
		t.Fatalf("a case needs a requester: got %v", err)
	}
}

func TestCases_ListingFiltersBySubjectAndStatus(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	a, err := mgr.OpenCase(ctx, "amina", mwanachamataskmanager.Case{SubjectKind: "market", SubjectID: "offer-1"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := mgr.OpenCase(ctx, "amina", mwanachamataskmanager.Case{SubjectKind: "form", SubjectID: "form-9"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.EndCase(ctx, b.ID, mwanachamataskmanager.CaseStatusCancelled, ""); err != nil {
		t.Fatal(err)
	}

	bySubject, err := mgr.ListCases(ctx, mwanachamataskmanager.CaseFilter{SubjectKind: "market", SubjectID: "offer-1"})
	if err != nil || len(bySubject) != 1 || bySubject[0].ID != a.ID {
		t.Fatalf("by subject: %+v %v", bySubject, err)
	}
	byStatus, err := mgr.ListCases(ctx, mwanachamataskmanager.CaseFilter{Status: mwanachamataskmanager.CaseStatusCancelled})
	if err != nil || len(byStatus) != 1 || byStatus[0].ID != b.ID {
		t.Fatalf("by status: %+v %v", byStatus, err)
	}
	all, err := mgr.ListCases(ctx, mwanachamataskmanager.CaseFilter{})
	if err != nil || len(all) != 2 {
		t.Fatalf("unfiltered: %d %v", len(all), err)
	}
}

func TestCases_AnIssueServesOneCaseAtATimeAndOnlyAnOpenCase(t *testing.T) {
	ctx := context.Background()
	mgr := newTestManager(t)
	first := openCaseBy(t, mgr, "amina")
	second := openCaseBy(t, mgr, "otieno")
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "shared issue"})

	if err := mgr.LinkCaseTask(ctx, first.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := mgr.LinkCaseTask(ctx, first.ID, task.ID); err != nil {
		t.Fatalf("linking the same issue to the same case twice is fine: %v", err)
	}
	if err := mgr.LinkCaseTask(ctx, second.ID, task.ID); !errors.Is(err, mwanachamataskmanager.ErrTaskAlreadyInCase) {
		t.Fatalf("an issue already in another case: got %v", err)
	}
	if err := mgr.LinkCaseTask(ctx, second.ID, "missing-task"); !errors.Is(err, mwanachamataskmanager.ErrTaskNotFound) {
		t.Fatalf("a missing issue: got %v", err)
	}
	if err := mgr.LinkCaseTask(ctx, "missing", task.ID); !errors.Is(err, mwanachamataskmanager.ErrCaseNotFound) {
		t.Fatalf("a missing case: got %v", err)
	}
	if _, err := mgr.EndCase(ctx, second.ID, mwanachamataskmanager.CaseStatusCancelled, ""); err != nil {
		t.Fatal(err)
	}
	other := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: "late issue"})
	if err := mgr.LinkCaseTask(ctx, second.ID, other.ID); !errors.Is(err, mwanachamataskmanager.ErrCaseNotOpen) {
		t.Fatalf("linking to a closed case: got %v", err)
	}
	tasks, err := mgr.ListCaseTasks(ctx, first.ID)
	if err != nil || len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("the first case's issues: %+v %v", tasks, err)
	}
}
