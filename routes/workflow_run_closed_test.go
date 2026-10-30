package routes_test

import (
	"encoding/json"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestAClosedOrderIsListedWithItsCancelRunOverTheMux(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)

	none := w73Do(t, m, "manager-1", "GET", "/workflow-runs/closed", nil)
	var empty []models.WorkflowRun
	if none.Code != 200 || json.Unmarshal(none.Body.Bytes(), &empty) != nil || len(empty) != 0 {
		t.Fatalf("closed before any %d %s", none.Code, none.Body)
	}

	started := w73Do(t, m, "manager-1", "POST", "/workflow-runs/start", map[string]any{"workflow_code": "ride-ask", "workflow_version": 3})
	var run models.WorkflowRun
	if started.Code != 201 || json.Unmarshal(started.Body.Bytes(), &run) != nil {
		t.Fatalf("start %d %s", started.Code, started.Body)
	}
	w73Do(t, m, "manager-1", "POST", "/workflow-runs/"+run.ID+"/step", map[string]any{"step_code": "WI-7", "limit_minutes": 5, "cancel_workflow_code": "ride-ask-cancel"})
	w73Do(t, m, "manager-1", "POST", "/workflow-runs/close-past-limit", map[string]any{"as_of": time.Now().UTC().Add(6 * time.Minute).Format(time.RFC3339)})

	listed := w73Do(t, m, "manager-1", "GET", "/workflow-runs/closed?workflow_code=ride-ask", nil)
	var runs []models.WorkflowRun
	if listed.Code != 200 || json.Unmarshal(listed.Body.Bytes(), &runs) != nil || len(runs) != 1 {
		t.Fatalf("closed %d %s", listed.Code, listed.Body)
	}
	got := runs[0]
	if got.ID != run.ID || got.Status != mwanachamataskmanager.WorkflowRunStatusClosed || got.CancelRunID == "" || got.CompletedAt == "" || got.WorkflowVersion != 3 {
		t.Errorf("closed run = %+v", got)
	}

	other := w73Do(t, m, "manager-1", "GET", "/workflow-runs/closed?workflow_code=buy-now", nil)
	var elsewhere []models.WorkflowRun
	if other.Code != 200 || json.Unmarshal(other.Body.Bytes(), &elsewhere) != nil || len(elsewhere) != 0 {
		t.Fatalf("closed of another workflow %d %s", other.Code, other.Body)
	}

	underWay := w73Do(t, m, "manager-1", "GET", "/workflow-runs/under-way", nil)
	var going []models.WorkflowRun
	if underWay.Code != 200 || json.Unmarshal(underWay.Body.Bytes(), &going) != nil || len(going) != 1 || going[0].ID != got.CancelRunID {
		t.Fatalf("under way after closing %d %s", underWay.Code, underWay.Body)
	}
}
