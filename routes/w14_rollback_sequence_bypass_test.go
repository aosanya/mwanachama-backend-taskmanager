package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/routes"
)

// wk14Manager/wk14Mux/wk14JSON mirror w11/w12's own local real-mux harness,
// duplicated so this file stands alone as the reproduction for board row W14.
func wk14Manager(t *testing.T, prefix string) mwanachamataskmanager.TaskManager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	tables := mwanachamataskmanager.DefaultTableNames(prefix)
	if err := mwanachamataskmanager.Migrate(db, tables); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, tables, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr
}

func wk14Mux(tm mwanachamataskmanager.TaskManager) *http.ServeMux {
	m := http.NewServeMux()
	for _, rt := range routes.Routes(tm) {
		m.HandleFunc(rt.Pattern(""), rt.Handler)
	}
	return m
}

func wk14JSON(t *testing.T, v map[string]any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func wk14Do(t *testing.T, m *http.ServeMux, method, path string, body *bytes.Reader) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	return rec
}

// Pins board row W14, half 1: DELETE /workflow-runs/{runID}/artifacts is
// registered as its own top-level route (routes/workflowrun_lifecycle.go)
// and calls TaskManager.DeleteWorkflowRunArtifacts directly, which never
// checks the run's own status (workflow_run_rollback.go's
// DeleteWorkflowRunArtifacts only calls GetWorkflowRun to confirm the run
// exists — no CanTransitionTo/status guard). So a caller can wipe a run's
// Task/TaskTodo artifacts on a run that never entered rolling_back at all,
// while the run's own status keeps reporting whatever it was — here,
// in_progress — throughout. RollbackWorkflowRun's documented "4-step
// compensation sequence" (rolling_back → cross-service → own artifacts →
// rolled_back) is a convention only ONE caller (RollbackWorkflowRun itself)
// follows; this route is an equally-privileged, independent entry point
// into step 3 alone.
func TestWorkflowRun_DeleteArtifactsBypassesRollbackStateMachine(t *testing.T) {
	tm := wk14Manager(t, "wk14a")
	m := wk14Mux(tm)

	createRun := wk14Do(t, m, "POST", "/workflow-runs", wk14JSON(t, map[string]any{"name": "w14-live-run"}))
	if createRun.Code != http.StatusCreated {
		t.Fatalf("create run: got %d, body %s", createRun.Code, createRun.Body.String())
	}
	var run struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createRun.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}

	if rec := wk14Do(t, m, "PUT", "/workflow-runs/"+run.ID+"/status", wk14JSON(t, map[string]any{"new_status": "in_progress"})); rec.Code != http.StatusOK {
		t.Fatalf("run -> in_progress: got %d, body %s", rec.Code, rec.Body.String())
	}

	createTask := wk14Do(t, m, "POST", "/tasks", wk14JSON(t, map[string]any{"title": "w14 task"}))
	if createTask.Code != http.StatusCreated {
		t.Fatalf("create task: got %d, body %s", createTask.Code, createTask.Body.String())
	}
	var task struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createTask.Body.Bytes(), &task); err != nil {
		t.Fatalf("decode task: %v", err)
	}

	if rec := wk14Do(t, m, "POST", "/workflow-runs/"+run.ID+"/tasks/"+task.ID+"/link", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("link task to run: got %d, body %s", rec.Code, rec.Body.String())
	}

	// Sanity: the run is genuinely in_progress, never touched rolling_back.
	before := wk14Do(t, m, "GET", "/workflow-runs/"+run.ID, nil)
	var beforeBody map[string]any
	json.Unmarshal(before.Body.Bytes(), &beforeBody)
	if beforeBody["status"] != "in_progress" {
		t.Fatalf("setup: run status = %v, want in_progress", beforeBody["status"])
	}

	// W14 pin: call DELETE artifacts directly, bypassing /rollback entirely.
	deleteArtifacts := wk14Do(t, m, "DELETE", "/workflow-runs/"+run.ID+"/artifacts", nil)
	if deleteArtifacts.Code != http.StatusNoContent {
		t.Fatalf("W14 pin violated: DELETE artifacts on an in_progress run was refused (got %d, body %s) — if this route now requires rolling_back, W14 may be fixed; update this test",
			deleteArtifacts.Code, deleteArtifacts.Body.String())
	}

	// The task WAS reset/unlinked — the destructive half of rollback ran.
	afterTask := wk14Do(t, m, "GET", "/tasks/"+task.ID, nil)
	var afterTaskBody map[string]any
	json.Unmarshal(afterTask.Body.Bytes(), &afterTaskBody)
	if afterTaskBody["status"] != "pending" {
		t.Errorf("task status after DELETE artifacts = %v, want pending (reset)", afterTaskBody["status"])
	}
	if wfid, ok := afterTaskBody["workflow_run_id"]; ok && wfid != "" {
		t.Errorf("task.workflow_run_id after DELETE artifacts = %v, want cleared", wfid)
	}
	listAfter := wk14Do(t, m, "GET", "/workflow-runs/"+run.ID+"/tasks", nil)
	var afterList []map[string]any
	json.Unmarshal(listAfter.Body.Bytes(), &afterList)
	if len(afterList) != 0 {
		t.Errorf("ListTasksForRun after DELETE artifacts = %d tasks, want 0 (unlinked)", len(afterList))
	}

	// W14 pin: but the run's OWN status never moved — no rolling_back, no
	// rolled_back, no run.* lifecycle event at all. The run still claims
	// in_progress despite its artifacts having just been wiped out from
	// under it.
	after := wk14Do(t, m, "GET", "/workflow-runs/"+run.ID, nil)
	var afterBody map[string]any
	json.Unmarshal(after.Body.Bytes(), &afterBody)
	if afterBody["status"] != "in_progress" {
		t.Fatalf("W14 pin violated: run status after DELETE artifacts = %v, want still in_progress (unchanged) — if the route now transitions the run's status as a side effect, W14 may be fixed; update this test",
			afterBody["status"])
	}
}

// Pins board row W14, half 2: PUT /workflow-runs/{runID}/status lets a
// caller drive a run straight from rolling_back to the TERMINAL rolled_back
// status (a transition [WorkflowRunStatus.CanTransitionTo] legally allows)
// without ever calling DeleteWorkflowRunArtifacts — so a run can report
// itself "rolled_back" (implying its artifacts were compensated) while its
// linked Task was never reset and never unlinked. Combined with the first
// half above, this shows RollbackWorkflowRun's compensation sequence
// (workflow_run_rollback.go's own doc comment: "Sequence: rolling_back →
// compensate cross-service artifacts → compensate own artifacts →
// rolled_back") is enforced by NEITHER of the two routes its steps are
// individually built from — only by RollbackWorkflowRun's own single Go
// caller choosing to call them in order. Any other path to the same two
// primitives — direct route calls, a retry script, an operator mistake —
// can produce "rolled_back but artifacts intact" or "in_progress but
// artifacts wiped" with no error and no warning.
func TestWorkflowRun_ForcedStatusToRolledBackSkipsArtifactCleanup(t *testing.T) {
	tm := wk14Manager(t, "wk14b")
	m := wk14Mux(tm)

	createRun := wk14Do(t, m, "POST", "/workflow-runs", wk14JSON(t, map[string]any{"name": "w14-force-run"}))
	var run struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createRun.Body.Bytes(), &run)

	wk14Do(t, m, "PUT", "/workflow-runs/"+run.ID+"/status", wk14JSON(t, map[string]any{"new_status": "in_progress"}))

	createTask := wk14Do(t, m, "POST", "/tasks", wk14JSON(t, map[string]any{"title": "w14b task"}))
	var task struct {
		ID string `json:"id"`
	}
	json.Unmarshal(createTask.Body.Bytes(), &task)
	if rec := wk14Do(t, m, "POST", "/workflow-runs/"+run.ID+"/tasks/"+task.ID+"/link", nil); rec.Code != http.StatusNoContent {
		t.Fatalf("link task to run: got %d, body %s", rec.Code, rec.Body.String())
	}

	if rec := wk14Do(t, m, "PUT", "/workflow-runs/"+run.ID+"/status", wk14JSON(t, map[string]any{"new_status": "failed"})); rec.Code != http.StatusOK {
		t.Fatalf("run -> failed: got %d, body %s", rec.Code, rec.Body.String())
	}
	// Simulate RollbackWorkflowRun's step 1 (acquire the lock) having
	// committed, then the caller going straight for step 4 without ever
	// invoking DeleteWorkflowRunArtifacts (step 3) — e.g. a crash-recovery
	// script that only knows how to "finish" a stuck rolling_back run.
	if rec := wk14Do(t, m, "PUT", "/workflow-runs/"+run.ID+"/status", wk14JSON(t, map[string]any{"new_status": "rolling_back"})); rec.Code != http.StatusOK {
		t.Fatalf("run -> rolling_back: got %d, body %s", rec.Code, rec.Body.String())
	}

	// W14 pin: force straight to rolled_back, skipping DeleteWorkflowRunArtifacts.
	forceRolledBack := wk14Do(t, m, "PUT", "/workflow-runs/"+run.ID+"/status", wk14JSON(t, map[string]any{"new_status": "rolled_back"}))
	if forceRolledBack.Code != http.StatusOK {
		t.Fatalf("W14 pin violated: PUT status rolling_back -> rolled_back was refused (got %d, body %s) — if this transition is now blocked without artifact cleanup having run, W14 may be fixed; update this test",
			forceRolledBack.Code, forceRolledBack.Body.String())
	}
	var forced map[string]any
	json.Unmarshal(forceRolledBack.Body.Bytes(), &forced)
	if forced["status"] != "rolled_back" {
		t.Fatalf("run status after forced transition = %v, want rolled_back", forced["status"])
	}

	// W14 pin: the task was NEVER reset/unlinked — the run claims
	// "rolled_back" (implying compensation happened) while its artifacts
	// are untouched.
	afterTask := wk14Do(t, m, "GET", "/tasks/"+task.ID, nil)
	var afterTaskBody map[string]any
	json.Unmarshal(afterTask.Body.Bytes(), &afterTaskBody)
	if afterTaskBody["workflow_run_id"] != run.ID {
		t.Fatalf("W14 pin violated: task.workflow_run_id after forced rolled_back = %v, want still %q (artifacts were NOT actually compensated) — if a status transition to rolled_back now triggers artifact cleanup as a side effect, W14 may be fixed; update this test",
			afterTaskBody["workflow_run_id"], run.ID)
	}
}
