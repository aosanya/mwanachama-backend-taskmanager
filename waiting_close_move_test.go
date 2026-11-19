package mwanachamataskmanager_test

import (
	"context"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"net/http"
	"strings"
	"testing"
)

func waitingIssueInBacklog(t *testing.T, mgr mwanachamataskmanager.TaskManager, backlog, title string, tags ...string) mwanachamataskmanager.Task {
	t.Helper()
	task := mustCreate(t, mgr, mwanachamataskmanager.Task{Title: title, Tags: tags})
	if _, err := mgr.MoveTaskToStep(context.Background(), task.ID, backlog, "to-do"); err != nil {
		t.Fatalf("MoveTaskToStep to-do: %v", err)
	}
	return task
}
func moveInBacklog(t *testing.T, mgr mwanachamataskmanager.TaskManager, backlog, id, step string) {
	t.Helper()
	if _, err := mgr.MoveTaskToStep(context.Background(), id, backlog, step); err != nil {
		t.Fatalf("MoveTaskToStep %s: %v", step, err)
	}
}
func issueCode(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) string {
	t.Helper()
	task, err := mgr.GetTask(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Code == "" {
		t.Fatal("the issue has no code to name")
	}
	return task.Code
}
func workerEntries(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) []string {
	t.Helper()
	var workers []string
	for _, entry := range logTexts(t, mgr, id) {
		if strings.HasPrefix(entry, "worker: ") {
			workers = append(workers, entry)
		}
	}
	return workers
}
func wantOneWorkerEntryNaming(t *testing.T, mgr mwanachamataskmanager.TaskManager, id, code string) {
	t.Helper()
	got := workerEntries(t, mgr, id)
	if len(got) != 1 || !strings.Contains(got[0], code) {
		t.Fatalf("worker log entries = %v, want one naming %s", got, code)
	}
}
func wantNoWorkerEntry(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) {
	t.Helper()
	if got := workerEntries(t, mgr, id); len(got) != 0 {
		t.Fatalf("worker log entries = %v, want none", got)
	}
}
func TestWaitingCloseMove_LastDependencyMovedClearsWaiting(t *testing.T) {
	for _, closing := range []string{"done", "archive", "cancelled"} {
		t.Run(closing, func(t *testing.T) {
			mgr := newTestManager(t)
			backlog := closingBacklog(t, mgr)
			dependent := waitingIssueInBacklog(t, mgr, backlog, "dependent", "Waiting", "hub-steward-swe")
			first := waitingIssueInBacklog(t, mgr, backlog, "first", "Ready")
			last := waitingIssueInBacklog(t, mgr, backlog, "last", "Ready")
			linkDependsOn(t, mgr, dependent, first)
			linkDependsOn(t, mgr, dependent, last)
			moveInBacklog(t, mgr, backlog, first.ID, closing)
			wantTags(t, mgr, dependent.ID, "Waiting", "hub-steward-swe")
			wantNoWorkerEntry(t, mgr, dependent.ID)
			moveInBacklog(t, mgr, backlog, last.ID, closing)
			wantTags(t, mgr, dependent.ID, "Ready", "hub-steward-swe")
			wantOneWorkerEntryNaming(t, mgr, dependent.ID, issueCode(t, mgr, last.ID))
		})
	}
}
func TestWaitingCloseMove_LastDependencyMovedFromReviewClearsWaiting(t *testing.T) {
	mgr := newTestManager(t)
	backlog := closingBacklog(t, mgr)
	dependent := waitingIssueInBacklog(t, mgr, backlog, "dependent", "Waiting")
	gate := inStep(t, mgr, backlog, "review", "gate")
	linkDependsOn(t, mgr, dependent, gate)
	moveInBacklog(t, mgr, backlog, gate.ID, "done")
	wantTags(t, mgr, dependent.ID, "Ready")
	wantOneWorkerEntryNaming(t, mgr, dependent.ID, issueCode(t, mgr, gate.ID))
}
func TestWaitingCloseMove_OneOpenDependencyKeepsWaitingAndLogsNothing(t *testing.T) {
	mgr := newTestManager(t)
	backlog := closingBacklog(t, mgr)
	dependent := waitingIssueInBacklog(t, mgr, backlog, "dependent", "Waiting")
	open := waitingIssueInBacklog(t, mgr, backlog, "open", "Ready")
	linkDependsOn(t, mgr, dependent, open)
	other := waitingIssueInBacklog(t, mgr, backlog, "other", "Ready")
	moveInBacklog(t, mgr, backlog, other.ID, "done")
	moveInBacklog(t, mgr, backlog, open.ID, "review")
	moveInBacklog(t, mgr, backlog, open.ID, "in-progress")
	wantTags(t, mgr, dependent.ID, "Waiting")
	wantNoWorkerEntry(t, mgr, dependent.ID)
}
func TestWaitingCloseMove_WaitingWithoutADependsOnEdgeSurvivesAnotherIssueClosing(t *testing.T) {
	mgr := newTestManager(t)
	backlog := closingBacklog(t, mgr)
	waitsByHand := waitingIssueInBacklog(t, mgr, backlog, "waits by hand", "Waiting", "hub-steward-swe")
	unrelated := waitingIssueInBacklog(t, mgr, backlog, "unrelated", "Ready")
	moveInBacklog(t, mgr, backlog, unrelated.ID, "done")
	wantTags(t, mgr, waitsByHand.ID, "Waiting", "hub-steward-swe")
	wantNoWorkerEntry(t, mgr, waitsByHand.ID)
}
func TestWaitingCloseMove_TodoListOfTheDependentIsLeftAlone(t *testing.T) {
	f := newTodoFixture(t)
	backlog := closingBacklog(t, f.mgr)
	dependent := waitingIssueInBacklog(t, f.mgr, backlog, "dependent", "Waiting")
	gate := waitingIssueInBacklog(t, f.mgr, backlog, "gate", "Ready")
	linkDependsOn(t, f.mgr, dependent, gate)
	first := f.piece(t, dependent.ID, "first", 1)
	second := f.piece(t, dependent.ID, "second", 2, first.ID)
	before := todoStatuses(t, f, dependent.ID)
	moveInBacklog(t, f.mgr, backlog, gate.ID, "done")
	wantTags(t, f.mgr, dependent.ID, "Ready")
	after := todoStatuses(t, f, dependent.ID)
	if before[first.ID] != after[first.ID] || before[second.ID] != after[second.ID] {
		t.Fatalf("todo statuses %v became %v when the Waiting tag cleared", before, after)
	}
	if len(after) != 2 {
		t.Fatalf("todos = %v, want the same two", after)
	}
}
func todoStatuses(t *testing.T, f todoFixture, issue string) map[string]string {
	t.Helper()
	list, err := f.mgr.ListTodosOfIssue(context.Background(), "", issue)
	if err != nil {
		t.Fatalf("ListTodosOfIssue: %v", err)
	}
	out := map[string]string{}
	for _, todo := range list.Items {
		out[todo.ID] = todo.State + "/" + string(todo.Status)
	}
	return out
}
func TestWaitingCloseMove_OverHTTPLastDependencyClearsWaiting(t *testing.T) {
	for _, closing := range []string{"done", "archive", "cancelled"} {
		t.Run(closing, func(t *testing.T) {
			h := newSendHub(t)
			h.addBacklog("work", "Work", []string{"To do", "In progress", "Review", "Done", "Archive", "Cancelled"})
			backlog := h.backlog["work"]
			dependentID := h.issueAt("work", "to-do", "dependent")
			firstID := h.issueAt("work", "to-do", "first")
			lastID := h.issueAt("work", "to-do", "last")
			h.expect(http.StatusOK, "owner", http.MethodPatch, "/tasks/"+dependentID, map[string]any{"tags": []string{"Waiting", "hub-steward-swe"}})
			for _, id := range []string{firstID, lastID} {
				linkDependsOn(t, h.mgr, mwanachamataskmanager.Task{ID: dependentID}, mwanachamataskmanager.Task{ID: id})
			}
			wantTags(t, h.mgr, dependentID, "Waiting", "hub-steward-swe")
			h.expect(http.StatusOK, "owner", http.MethodPost, "/tasks/"+firstID+"/move-to-step", map[string]any{"backlog": backlog, "step": closing})
			wantTags(t, h.mgr, dependentID, "Waiting", "hub-steward-swe")
			wantNoWorkerEntry(t, h.mgr, dependentID)
			h.expect(http.StatusOK, "owner", http.MethodPost, "/tasks/"+lastID+"/move-to-step", map[string]any{"backlog": backlog, "step": closing})
			wantTags(t, h.mgr, dependentID, "Ready", "hub-steward-swe")
			wantOneWorkerEntryNaming(t, h.mgr, dependentID, issueCode(t, h.mgr, lastID))
		})
	}
}
