package routes_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aosanya/mwanachama-backend-shared/spec"

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
	workSpec, _ := specForInstance(t, "../spec/examples/work.taskmanager.json", prefix)
	if err := spec.Migrate(db, workSpec); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	mgr, err := mwanachamataskmanager.NewTaskManager(db, workSpec, nil)
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

// Guards board row W14, half 1, fixed 2026-09-28.
// DELETE /workflow-runs/{runID}/artifacts is its own top-level route over
// step 3 of the rollback sequence, and DeleteWorkflowRunArtifacts used to
// check only that the run existed — so a caller could wipe a live run's
// Task/TaskTodo artifacts while the run's own status kept reporting
// in_progress throughout. Compensation now requires the run to be in
// rolling_back, which only RollbackWorkflowRun puts it in.
//
// This asserted the bypass until the fix; it now asserts the refusal and
// that the live run's artifacts are untouched by it.
func TestWorkflowRun_DeleteArtifactsIsRefusedOnARunNobodyIsRollingBack(t *testing.T) {
	tm := wk14Manager(t, "wk14a")
	mux := wk14Mux(tm)

	runResp := wk14Do(t, mux, "POST", "/workflow-runs", wk14JSON(t, map[string]any{"name": "w14-live-run"}))
	if runResp.Code != http.StatusCreated {
		t.Fatalf("create run: got %d, body %s", runResp.Code, runResp.Body.String())
	}
	var run map[string]any
	if err := json.Unmarshal(runResp.Body.Bytes(), &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	runID, _ := run["id"].(string)

	if r := wk14Do(t, mux, "PUT", "/workflow-runs/"+runID+"/status",
		wk14JSON(t, map[string]any{"new_status": "in_progress"})); r.Code != http.StatusOK {
		t.Fatalf("run -> in_progress: got %d, body %s", r.Code, r.Body.String())
	}

	taskResp := wk14Do(t, mux, "POST", "/tasks", wk14JSON(t, map[string]any{
		"title": "live work", "workflow_run_id": runID,
	}))
	if taskResp.Code != http.StatusCreated {
		t.Fatalf("create task: got %d, body %s", taskResp.Code, taskResp.Body.String())
	}
	var task map[string]any
	if err := json.Unmarshal(taskResp.Body.Bytes(), &task); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	taskID, _ := task["id"].(string)

	del := wk14Do(t, mux, "DELETE", "/workflow-runs/"+runID+"/artifacts", nil)
	if del.Code != http.StatusConflict {
		t.Errorf("DELETE artifacts on an in_progress run: got %d, body %s, want 409",
			del.Code, del.Body.String())
	}

	// The refused call left the run's work exactly where it was.
	tasksResp := wk14Do(t, mux, "GET", "/workflow-runs/"+runID+"/tasks", nil)
	var stillLinked []map[string]any
	if err := json.Unmarshal(tasksResp.Body.Bytes(), &stillLinked); err != nil {
		t.Fatalf("unmarshal run tasks: %v", err)
	}
	if len(stillLinked) != 1 {
		t.Errorf("the run holds %d task(s), want 1 — the refused call compensated anyway: %s",
			len(stillLinked), tasksResp.Body.String())
	}

	readResp := wk14Do(t, mux, "GET", "/tasks/"+taskID, nil)
	var after map[string]any
	if err := json.Unmarshal(readResp.Body.Bytes(), &after); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	if after["workflow_run_id"] != runID {
		t.Errorf("task.workflow_run_id = %v, want it still anchored to the run", after["workflow_run_id"])
	}
	if after["status"] != "pending" {
		t.Errorf("task.status = %v, want pending (unchanged)", after["status"])
	}
}

// Guards board row W14, half 2, fixed 2026-09-28.
// PUT /workflow-runs/{runID}/status is its own top-level route over step 4,
// and rolling_back → rolled_back was a legal transition, so a caller could
// drive a run to the terminal rolled_back status without any compensation
// having run — leaving tasks still anchored to a run reporting itself undone.
// The three states a rollback drives a run through are now the coordinator's,
// and the public setter refuses all of them.
func TestWorkflowRun_StatusRouteRefusesTheRollbackStates(t *testing.T) {
	tm := wk14Manager(t, "wk14b")
	mux := wk14Mux(tm)

	runResp := wk14Do(t, mux, "POST", "/workflow-runs", wk14JSON(t, map[string]any{"name": "w14-forced-run"}))
	if runResp.Code != http.StatusCreated {
		t.Fatalf("create run: got %d, body %s", runResp.Code, runResp.Body.String())
	}
	var run map[string]any
	if err := json.Unmarshal(runResp.Body.Bytes(), &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	runID, _ := run["id"].(string)

	for _, s := range []string{"in_progress", "failed"} {
		if r := wk14Do(t, mux, "PUT", "/workflow-runs/"+runID+"/status",
			wk14JSON(t, map[string]any{"new_status": s})); r.Code != http.StatusOK {
			t.Fatalf("run -> %s: got %d, body %s", s, r.Code, r.Body.String())
		}
	}

	for _, s := range []string{"rolling_back", "rolled_back", "rollback_failed"} {
		r := wk14Do(t, mux, "PUT", "/workflow-runs/"+runID+"/status",
			wk14JSON(t, map[string]any{"new_status": s}))
		if r.Code != http.StatusBadRequest {
			t.Errorf("setting %s: got %d, body %s, want 400", s, r.Code, r.Body.String())
		}
	}

	get := wk14Do(t, mux, "GET", "/workflow-runs/"+runID, nil)
	var after map[string]any
	if err := json.Unmarshal(get.Body.Bytes(), &after); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	if after["status"] != "failed" {
		t.Errorf("run status = %v, want failed — a refused set moved it anyway", after["status"])
	}
}

// The sequence still runs end to end through its one entry point: the run
// reaches rolled_back and its artifacts really were compensated.
func TestWorkflowRun_RollbackCompensatesAndFinalizesTogether(t *testing.T) {
	tm := wk14Manager(t, "wk14c")
	mux := wk14Mux(tm)

	runResp := wk14Do(t, mux, "POST", "/workflow-runs", wk14JSON(t, map[string]any{"name": "w14-whole-run"}))
	var run map[string]any
	if err := json.Unmarshal(runResp.Body.Bytes(), &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	runID, _ := run["id"].(string)

	for _, s := range []string{"in_progress", "failed"} {
		if r := wk14Do(t, mux, "PUT", "/workflow-runs/"+runID+"/status",
			wk14JSON(t, map[string]any{"new_status": s})); r.Code != http.StatusOK {
			t.Fatalf("run -> %s: got %d, body %s", s, r.Code, r.Body.String())
		}
	}

	taskResp := wk14Do(t, mux, "POST", "/tasks", wk14JSON(t, map[string]any{
		"title": "work to undo", "workflow_run_id": runID,
	}))
	var task map[string]any
	if err := json.Unmarshal(taskResp.Body.Bytes(), &task); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	taskID, _ := task["id"].(string)

	roll := wk14Do(t, mux, "POST", "/workflow-runs/"+runID+"/rollback",
		wk14JSON(t, map[string]any{"reason": "undo it"}))
	if roll.Code != http.StatusOK {
		t.Fatalf("rollback: got %d, body %s", roll.Code, roll.Body.String())
	}
	var rolled map[string]any
	if err := json.Unmarshal(roll.Body.Bytes(), &rolled); err != nil {
		t.Fatalf("unmarshal rollback: %v", err)
	}
	if rolled["status"] != "rolled_back" {
		t.Errorf("status = %v, want rolled_back", rolled["status"])
	}

	readResp := wk14Do(t, mux, "GET", "/tasks/"+taskID, nil)
	var after map[string]any
	if err := json.Unmarshal(readResp.Body.Bytes(), &after); err != nil {
		t.Fatalf("unmarshal task: %v", err)
	}
	if after["workflow_run_id"] != "" && after["workflow_run_id"] != nil {
		t.Errorf("task.workflow_run_id = %v, want cleared — the run says rolled_back, so its work must really be undone",
			after["workflow_run_id"])
	}
}
