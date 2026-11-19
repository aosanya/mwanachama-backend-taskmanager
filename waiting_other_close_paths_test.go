package mwanachamataskmanager_test

import (
	"context"
	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"net/http"
	"testing"
)

var closingStatuses = []mwanachamataskmanager.TaskStatus{mwanachamataskmanager.TaskStatusCompleted, mwanachamataskmanager.TaskStatusCancelled}
var closingStateNames = map[mwanachamataskmanager.TaskStatus]string{mwanachamataskmanager.TaskStatusCompleted: "Done", mwanachamataskmanager.TaskStatusCancelled: "Cancelled"}

func startWorking(t *testing.T, mgr mwanachamataskmanager.TaskManager, id string) {
	t.Helper()
	if _, err := mgr.MoveTaskToState(context.Background(), id, "Doing"); err != nil {
		t.Fatalf("MoveTaskToState Doing: %v", err)
	}
}
func wantFreedByLastClose(t *testing.T, mgr mwanachamataskmanager.TaskManager, closeIssue func(id string)) {
	t.Helper()
	backlog := closingBacklog(t, mgr)
	dependent := waitingIssueInBacklog(t, mgr, backlog, "dependent", "Waiting", "hub-steward-swe")
	first := waitingIssueInBacklog(t, mgr, backlog, "first", "Ready")
	last := waitingIssueInBacklog(t, mgr, backlog, "last", "Ready")
	linkDependsOn(t, mgr, dependent, first)
	linkDependsOn(t, mgr, dependent, last)
	closeIssue(first.ID)
	wantTags(t, mgr, dependent.ID, "Waiting", "hub-steward-swe")
	wantNoWorkerEntry(t, mgr, dependent.ID)
	closeIssue(last.ID)
	wantTags(t, mgr, dependent.ID, "Ready", "hub-steward-swe")
	wantOneWorkerEntryNaming(t, mgr, dependent.ID, issueCode(t, mgr, last.ID))
}
func TestWaitingOtherClose_MoveTaskToStateFreesDependents(t *testing.T) {
	for _, status := range closingStatuses {
		t.Run(string(status), func(t *testing.T) {
			mgr := newTestManager(t)
			wantFreedByLastClose(t, mgr, func(id string) {
				if _, err := mgr.MoveTaskToState(context.Background(), id, closingStateNames[status]); err != nil {
					t.Fatalf("MoveTaskToState %s: %v", closingStateNames[status], err)
				}
			})
		})
	}
}
func TestWaitingOtherClose_PatchTaskStatusFreesDependents(t *testing.T) {
	for _, status := range closingStatuses {
		t.Run(string(status), func(t *testing.T) {
			mgr := newTestManager(t)
			wantFreedByLastClose(t, mgr, func(id string) {
				startWorking(t, mgr, id)
				patched := status
				if _, err := mgr.PatchTaskAs(context.Background(), "", mwanachamataskmanager.TaskPatch{ID: id, Status: &patched}); err != nil {
					t.Fatalf("PatchTaskAs %s: %v", status, err)
				}
			})
		})
	}
}
func TestWaitingOtherClose_PatchTaskFromPendingCancelFreesDependents(t *testing.T) {
	mgr := newTestManager(t)
	wantFreedByLastClose(t, mgr, func(id string) {
		cancelled := mwanachamataskmanager.TaskStatusCancelled
		if _, err := mgr.PatchTaskAs(context.Background(), "", mwanachamataskmanager.TaskPatch{ID: id, Status: &cancelled}); err != nil {
			t.Fatalf("PatchTaskAs cancelled: %v", err)
		}
	})
}
func TestWaitingOtherClose_UpdateTaskStatusFreesDependents(t *testing.T) {
	for _, status := range closingStatuses {
		t.Run(string(status), func(t *testing.T) {
			mgr := newTestManager(t)
			wantFreedByLastClose(t, mgr, func(id string) {
				startWorking(t, mgr, id)
				task, err := mgr.GetTask(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				task.Status = status
				if _, err := mgr.UpdateTask(context.Background(), task); err != nil {
					t.Fatalf("UpdateTask %s: %v", status, err)
				}
			})
		})
	}
}
func TestWaitingOtherClose_BatchPatchStatusFreesDependents(t *testing.T) {
	for _, status := range closingStatuses {
		for _, allOrNothing := range []bool{false, true} {
			name := string(status)
			if allOrNothing {
				name += "_all_or_nothing"
			}
			t.Run(name, func(t *testing.T) {
				mgr := newTestManager(t)
				wantFreedByLastClose(t, mgr, func(id string) {
					startWorking(t, mgr, id)
					patched := status
					out, err := mgr.Batch(context.Background(), "", mwanachamataskmanager.BatchRequest{AllOrNothing: allOrNothing, Operations: []mwanachamataskmanager.BatchOperation{{Op: "patch", TaskID: id, Patch: &mwanachamataskmanager.TaskPatch{Status: &patched}}}})
					if err != nil || out.Applied != 1 {
						t.Fatalf("Batch patch %s = %+v, %v, want one applied", status, out, err)
					}
				})
			})
		}
	}
}
func TestWaitingOtherClose_BatchMoveToStepFreesDependents(t *testing.T) {
	for _, step := range []string{"done", "archive", "cancelled"} {
		t.Run(step, func(t *testing.T) {
			mgr := newTestManager(t)
			var backlog string
			ctx := context.Background()
			wantFreedByLastClose(t, mgr, func(id string) {
				if backlog == "" {
					task, err := mgr.GetTask(ctx, id)
					if err != nil {
						t.Fatal(err)
					}
					backlog = task.Backlog
				}
				out, err := mgr.Batch(ctx, "", mwanachamataskmanager.BatchRequest{Operations: []mwanachamataskmanager.BatchOperation{{Op: "move_to_step", TaskID: id, Backlog: backlog, Step: step}}})
				if err != nil || out.Applied != 1 {
					t.Fatalf("Batch move to %s = %+v, %v, want one applied", step, out, err)
				}
			})
		})
	}
}
func TestWaitingOtherClose_BatchClosingTheLastTwoInOneCallFreesDependentOnce(t *testing.T) {
	mgr := newTestManager(t)
	ctx := context.Background()
	backlog := closingBacklog(t, mgr)
	dependent := waitingIssueInBacklog(t, mgr, backlog, "dependent", "Waiting")
	first := waitingIssueInBacklog(t, mgr, backlog, "first", "Ready")
	last := waitingIssueInBacklog(t, mgr, backlog, "last", "Ready")
	linkDependsOn(t, mgr, dependent, first)
	linkDependsOn(t, mgr, dependent, last)
	cancelled := mwanachamataskmanager.TaskStatusCancelled
	out, err := mgr.Batch(ctx, "", mwanachamataskmanager.BatchRequest{Operations: []mwanachamataskmanager.BatchOperation{{Op: "move_to_step", TaskID: first.ID, Backlog: backlog, Step: "done"}, {Op: "patch", TaskID: last.ID, Patch: &mwanachamataskmanager.TaskPatch{Status: &cancelled}}}})
	if err != nil || out.Applied != 2 {
		t.Fatalf("Batch = %+v, %v, want two applied", out, err)
	}
	wantTags(t, mgr, dependent.ID, "Ready")
	wantOneWorkerEntryNaming(t, mgr, dependent.ID, issueCode(t, mgr, last.ID))
}
func TestWaitingOtherClose_OverHTTPFreesDependents(t *testing.T) {
	closers := map[string]func(t *testing.T, h *sendHub, id string){"move_to_state_done": func(t *testing.T, h *sendHub, id string) {
		h.expect(http.StatusOK, "owner", http.MethodPost, "/tasks/"+id+"/move", map[string]any{"state": "Done"})
	}, "move_to_state_cancelled": func(t *testing.T, h *sendHub, id string) {
		h.expect(http.StatusOK, "owner", http.MethodPost, "/tasks/"+id+"/move", map[string]any{"state": "Cancelled"})
	}, "patch_completed": func(t *testing.T, h *sendHub, id string) {
		startWorking(t, h.mgr, id)
		h.expect(http.StatusOK, "owner", http.MethodPatch, "/tasks/"+id, map[string]any{"status": "completed"})
	}, "patch_cancelled": func(t *testing.T, h *sendHub, id string) {
		h.expect(http.StatusOK, "owner", http.MethodPatch, "/tasks/"+id, map[string]any{"status": "cancelled"})
	}, "update_completed": func(t *testing.T, h *sendHub, id string) {
		startWorking(t, h.mgr, id)
		body := h.expect(http.StatusOK, "owner", http.MethodGet, "/tasks/"+id, nil)
		body["status"] = "completed"
		h.expect(http.StatusOK, "owner", http.MethodPut, "/tasks/"+id, body)
	}, "update_cancelled": func(t *testing.T, h *sendHub, id string) {
		body := h.expect(http.StatusOK, "owner", http.MethodGet, "/tasks/"+id, nil)
		body["status"] = "cancelled"
		h.expect(http.StatusOK, "owner", http.MethodPut, "/tasks/"+id, body)
	}, "batch_patch_completed": func(t *testing.T, h *sendHub, id string) {
		startWorking(t, h.mgr, id)
		h.expect(http.StatusOK, "owner", http.MethodPost, "/batch", map[string]any{"operations": []any{map[string]any{"op": "patch", "task_id": id, "patch": map[string]any{"status": "completed"}}}})
	}, "batch_patch_cancelled": func(t *testing.T, h *sendHub, id string) {
		h.expect(http.StatusOK, "owner", http.MethodPost, "/batch", map[string]any{"operations": []any{map[string]any{"op": "patch", "task_id": id, "patch": map[string]any{"status": "cancelled"}}}})
	}, "batch_move_to_step_done": func(t *testing.T, h *sendHub, id string) {
		h.expect(http.StatusOK, "owner", http.MethodPost, "/batch", map[string]any{"operations": []any{map[string]any{"op": "move_to_step", "task_id": id, "backlog": h.backlog["work"], "step": "done"}}})
	}}
	for name, closeIssue := range closers {
		t.Run(name, func(t *testing.T) {
			h := newSendHub(t)
			h.addBacklog("work", "Work", []string{"To do", "In progress", "Review", "Done", "Archive", "Cancelled"})
			dependentID := h.issueAt("work", "to-do", "dependent")
			firstID := h.issueAt("work", "to-do", "first")
			lastID := h.issueAt("work", "to-do", "last")
			h.expect(http.StatusOK, "owner", http.MethodPatch, "/tasks/"+dependentID, map[string]any{"tags": []string{"Waiting", "hub-steward-swe"}})
			for _, id := range []string{firstID, lastID} {
				linkDependsOn(t, h.mgr, mwanachamataskmanager.Task{ID: dependentID}, mwanachamataskmanager.Task{ID: id})
			}
			closeIssue(t, h, firstID)
			wantTags(t, h.mgr, dependentID, "Waiting", "hub-steward-swe")
			wantNoWorkerEntry(t, h.mgr, dependentID)
			closeIssue(t, h, lastID)
			wantTags(t, h.mgr, dependentID, "Ready", "hub-steward-swe")
			wantOneWorkerEntryNaming(t, h.mgr, dependentID, issueCode(t, h.mgr, lastID))
		})
	}
}
