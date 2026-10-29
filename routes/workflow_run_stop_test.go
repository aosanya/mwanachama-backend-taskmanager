package routes_test

import (
	"encoding/json"
	"testing"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestAnOrderIsStartedListedAndStoppedOverTheMux(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)

	started := w73Do(t, m, "manager-1", "POST", "/workflow-runs/start", map[string]any{"workflow_code": "group-order", "workflow_version": 2})
	var run models.WorkflowRun
	if started.Code != 201 || json.Unmarshal(started.Body.Bytes(), &run) != nil || run.WorkflowVersion != 2 {
		t.Fatalf("start %d %s", started.Code, started.Body)
	}

	missing := w73Do(t, m, "manager-1", "POST", "/workflow-runs/start", map[string]any{"workflow_version": 1})
	if missing.Code != 400 {
		t.Fatalf("start without a workflow %d %s", missing.Code, missing.Body)
	}

	listed := w73Do(t, m, "manager-1", "GET", "/workflow-runs/under-way?workflow_code=group-order", nil)
	var runs []models.WorkflowRun
	if listed.Code != 200 || json.Unmarshal(listed.Body.Bytes(), &runs) != nil || len(runs) != 1 || runs[0].ID != run.ID || runs[0].WorkflowVersion != 2 {
		t.Fatalf("under way %d %s", listed.Code, listed.Body)
	}

	stopped := w73Do(t, m, "manager-1", "POST", "/workflow-runs/"+run.ID+"/stop", map[string]any{"reason": "wrong prices", "restart_on_version": 1})
	var stop models.WorkflowRunStop
	if stopped.Code != 200 || json.Unmarshal(stopped.Body.Bytes(), &stop) != nil {
		t.Fatalf("stop %d %s", stopped.Code, stopped.Body)
	}
	if stop.Stopped.Status != mwanachamataskmanager.WorkflowRunStatusCancelled || stop.Stopped.CancelledBy != "manager-1" {
		t.Errorf("stopped = %+v, want cancelled by the caller", stop.Stopped)
	}
	if stop.Restarted == nil || stop.Restarted.WorkflowVersion != 1 || stop.Restarted.RestartedFromRunID != run.ID {
		t.Errorf("restarted = %+v, want version 1 from the stopped run", stop.Restarted)
	}

	again := w73Do(t, m, "manager-1", "POST", "/workflow-runs/"+run.ID+"/stop", map[string]any{})
	if again.Code != 409 {
		t.Fatalf("stop twice %d %s", again.Code, again.Body)
	}
}
