package routes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

type callerKey struct{}

func w73Mux(tm mwanachamataskmanager.TaskManager) *http.ServeMux {
	m := http.NewServeMux()
	mount := routes.Mount{Caller: func(ctx context.Context) string {
		s, _ := ctx.Value(callerKey{}).(string)
		return s
	}}
	for _, rt := range routes.RoutesFor(tm, mount) {
		m.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	return m
}

func w73Do(t *testing.T, m *http.ServeMux, caller, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req = req.WithContext(context.WithValue(req.Context(), callerKey{}, caller))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	return rec
}

func w73Open(t *testing.T, m *http.ServeMux, caller string) mwanachamataskmanager.Case {
	t.Helper()
	rec := w73Do(t, m, caller, "POST", "/cases", map[string]any{"title": "pothole"})
	var c mwanachamataskmanager.Case
	if rec.Code != 201 || json.Unmarshal(rec.Body.Bytes(), &c) != nil {
		t.Fatalf("open %d %s", rec.Code, rec.Body)
	}
	return c
}

func TestACaseWithNoTasksStaysOpenUntilEndedDoneWithAReply(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)
	c := w73Open(t, m, "resident-1")
	rec := w73Do(t, m, "staff-1", "POST", "/cases/"+c.ID+"/end", map[string]any{"status": "done", "reply": "Fixed on Monday."})
	var ended mwanachamataskmanager.Case
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &ended) != nil {
		t.Fatalf("end %d %s", rec.Code, rec.Body)
	}
	if ended.Status != mwanachamataskmanager.CaseStatusDone || ended.Reply != "Fixed on Monday." || ended.ClosedAt == "" {
		t.Fatalf("ended %+v", ended)
	}
	again := w73Do(t, m, "staff-1", "POST", "/cases/"+c.ID+"/end", map[string]any{"status": "cancelled"})
	if again.Code != 409 {
		t.Fatalf("second end %d %s", again.Code, again.Body)
	}
}

func TestEndingACaseDoneIsRefusedWhileATaskIsOpenAndCancellingCancelsIt(t *testing.T) {
	ctx := context.Background()
	tm := wk11Manager(t)
	m := w73Mux(tm)
	c := w73Open(t, m, "resident-1")
	task, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tm.LinkCaseTask(ctx, c.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	done := w73Do(t, m, "staff-1", "POST", "/cases/"+c.ID+"/end", map[string]any{"status": "done"})
	if done.Code != 409 {
		t.Fatalf("done with open task %d %s", done.Code, done.Body)
	}
	bad := w73Do(t, m, "staff-1", "POST", "/cases/"+c.ID+"/end", map[string]any{"status": "open"})
	if bad.Code != 400 {
		t.Fatalf("end as open %d %s", bad.Code, bad.Body)
	}
	cancel := w73Do(t, m, "staff-1", "POST", "/cases/"+c.ID+"/end", map[string]any{"status": "cancelled", "reply": "Duplicate."})
	var ended mwanachamataskmanager.Case
	if cancel.Code != 200 || json.Unmarshal(cancel.Body.Bytes(), &ended) != nil || ended.Status != mwanachamataskmanager.CaseStatusCancelled {
		t.Fatalf("cancel %d %s", cancel.Code, cancel.Body)
	}
	got, err := tm.GetTask(ctx, task.ID)
	if err != nil || got.Status != mwanachamataskmanager.TaskStatusCancelled {
		t.Fatalf("task after cancel %+v %v", got, err)
	}
	after, err := tm.GetCase(ctx, c.ID)
	if err != nil || after.Status != mwanachamataskmanager.CaseStatusCancelled {
		t.Fatalf("case after cancel %+v %v", after, err)
	}
}

func TestMyCasesListsOnlyTheCallersOwnWhateverTheQueryNames(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)
	mine := w73Open(t, m, "resident-1")
	w73Open(t, m, "resident-2")
	rec := w73Do(t, m, "resident-1", "GET", "/cases/mine?requester_id=resident-2", nil)
	var listed []mwanachamataskmanager.Case
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &listed) != nil {
		t.Fatalf("mine %d %s", rec.Code, rec.Body)
	}
	if len(listed) != 1 || listed[0].ID != mine.ID {
		t.Fatalf("mine %+v", listed)
	}
	staff := w73Do(t, m, "staff-1", "GET", "/cases", nil)
	var all []mwanachamataskmanager.Case
	if staff.Code != 200 || json.Unmarshal(staff.Body.Bytes(), &all) != nil || len(all) != 2 {
		t.Fatalf("staff list %d %s", staff.Code, staff.Body)
	}
}

func TestDeletingACasesOnlyTaskLeavesTheCaseOpen(t *testing.T) {
	ctx := context.Background()
	tm := wk11Manager(t)
	m := w73Mux(tm)
	c := w73Open(t, m, "resident-1")
	task, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tm.LinkCaseTask(ctx, c.ID, task.ID); err != nil {
		t.Fatal(err)
	}
	if err := tm.DeleteTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	got, err := tm.GetCase(ctx, c.ID)
	if err != nil || got.Status != mwanachamataskmanager.CaseStatusOpen {
		t.Fatalf("case after deleting its only task %+v %v", got, err)
	}
}

func TestDeletingTheLastOpenTaskSettlesACaseWhoseOtherTaskIsDone(t *testing.T) {
	ctx := context.Background()
	tm := wk11Manager(t)
	m := w73Mux(tm)
	c := w73Open(t, m, "resident-1")
	finished, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	open, err := tm.CreateTask(ctx, mwanachamataskmanager.Task{Title: "repair"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{finished.ID, open.ID} {
		if err := tm.LinkCaseTask(ctx, c.ID, id); err != nil {
			t.Fatal(err)
		}
	}
	finished.Status = mwanachamataskmanager.TaskStatusCancelled
	if _, err := tm.UpdateTask(ctx, finished); err != nil {
		t.Fatal(err)
	}
	if err := tm.DeleteTask(ctx, open.ID); err != nil {
		t.Fatal(err)
	}
	got, err := tm.GetCase(ctx, c.ID)
	if err != nil || got.Status != mwanachamataskmanager.CaseStatusDone {
		t.Fatalf("case after deleting its last open task %+v %v", got, err)
	}
}
