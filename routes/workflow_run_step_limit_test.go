package routes_test

import (
	"encoding/json"
	"testing"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func TestAStepLimitIsBegunSweptAndClosedOverTheMux(t *testing.T) {
	tm := wk11Manager(t)
	m := w73Mux(tm)

	started := w73Do(t, m, "manager-1", "POST", "/workflow-runs/start", map[string]any{"workflow_code": "ride-ask", "workflow_version": 1})
	var run models.WorkflowRun
	if started.Code != 201 || json.Unmarshal(started.Body.Bytes(), &run) != nil {
		t.Fatalf("start %d %s", started.Code, started.Body)
	}

	refused := w73Do(t, m, "manager-1", "POST", "/workflow-runs/"+run.ID+"/step", map[string]any{"step_code": "WI-7", "limit_minutes": 5})
	if refused.Code != 400 {
		t.Fatalf("limit with no cancel workflow %d %s", refused.Code, refused.Body)
	}

	begun := w73Do(t, m, "manager-1", "POST", "/workflow-runs/"+run.ID+"/step", map[string]any{"step_code": "WI-7", "limit_minutes": 5, "cancel_workflow_code": "ride-ask-cancel"})
	var waiting models.WorkflowRun
	if begun.Code != 200 || json.Unmarshal(begun.Body.Bytes(), &waiting) != nil || waiting.StepDueAt == "" {
		t.Fatalf("begin %d %s", begun.Code, begun.Body)
	}

	early := w73Do(t, m, "manager-1", "POST", "/workflow-runs/close-past-limit", map[string]any{"as_of": time.Now().UTC().Format(time.RFC3339)})
	var none []models.WorkflowRun
	if early.Code != 200 || json.Unmarshal(early.Body.Bytes(), &none) != nil || len(none) != 0 {
		t.Fatalf("early sweep %d %s", early.Code, early.Body)
	}

	late := w73Do(t, m, "manager-1", "POST", "/workflow-runs/close-past-limit", map[string]any{"as_of": time.Now().UTC().Add(6 * time.Minute).Format(time.RFC3339)})
	var closed []models.WorkflowRun
	if late.Code != 200 || json.Unmarshal(late.Body.Bytes(), &closed) != nil || len(closed) != 1 {
		t.Fatalf("late sweep %d %s", late.Code, late.Body)
	}
	if closed[0].Status != mwanachamataskmanager.WorkflowRunStatusClosed || closed[0].CancelRunID == "" {
		t.Errorf("closed = %+v", closed[0])
	}

	answered := w73Do(t, m, "manager-1", "POST", "/workflow-runs/"+run.ID+"/step/answered", map[string]any{})
	if answered.Code != 200 {
		t.Fatalf("answered %d %s", answered.Code, answered.Body)
	}
}
