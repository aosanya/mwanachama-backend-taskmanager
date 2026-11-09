package routes_test

import (
	"net/http"
	"testing"
	"time"
)

type moreWritesFixture struct {
	h       *accessHub
	docs    string
	issue   string
	project string
	run     string
	kase    string
}

func newMoreWritesFixture(t *testing.T) *moreWritesFixture {
	t.Helper()
	h := newAccessHub(t, map[string][]string{"ann": {"contributors"}, "boss": {"owners"}})
	h.expect(http.StatusNoContent, "", "PUT", "/backlog-owner-group", map[string]string{"group_id": "owners"})
	f := &moreWritesFixture{h: h}
	f.docs = h.expect(http.StatusCreated, "boss", "POST", "/backlogs", map[string]any{"name": "Docs", "steps": []string{"Write", "Done"}})["id"].(string)
	f.issue = h.expect(http.StatusCreated, "boss", "POST", "/tasks", map[string]any{"title": "write the guide", "backlog": f.docs, "step": "write"})["id"].(string)
	f.project = h.expect(http.StatusCreated, "boss", "POST", "/projects", map[string]any{"name": "guide"})["id"].(string)
	f.run = h.expect(http.StatusCreated, "boss", "POST", "/workflow-runs", map[string]any{"name": "run", "trigger_event": "t", "initiator": "boss"})["id"].(string)
	h.expect(http.StatusOK, "boss", "PUT", "/workflow-runs/"+f.run+"/status", map[string]any{"new_status": "in_progress"})
	h.expect(http.StatusNoContent, "boss", "POST", "/workflow-runs/"+f.run+"/tasks/"+f.issue+"/link", nil)
	f.kase = h.expect(http.StatusCreated, "boss", "POST", "/cases", map[string]any{"subject_kind": "thing", "subject_id": "1"})["id"].(string)
	h.expect(http.StatusNoContent, "boss", "PUT", "/cases/"+f.kase+"/tasks/"+f.issue, nil)
	h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+f.docs+"/access/contributors", map[string]string{"level": "see"})
	return f
}

func (f *moreWritesFixture) refusedForAnn(t *testing.T) {
	t.Helper()
	h := f.h
	deadline := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+f.issue+"/assign", map[string]any{"agent_id": "worker"})
	h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+f.issue+"/unassign", nil)
	h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+f.issue+"/assign-holder", map[string]any{"assignee_kind": "actor", "assignee_id": "someone"})
	h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+f.issue+"/unblock", map[string]any{"note": "go"})
	h.expect(http.StatusForbidden, "ann", "POST", "/tasks/"+f.issue+"/unblock-dependents", nil)
	h.expect(http.StatusForbidden, "ann", "POST", "/projects/"+f.project+"/tasks/"+f.issue, nil)
	h.expect(http.StatusForbidden, "ann", "DELETE", "/projects/"+f.project+"/tasks/"+f.issue, nil)
	h.expect(http.StatusForbidden, "ann", "POST", "/projects/guide/tasks", map[string]any{"title": "new", "backlog": f.docs, "step": "write"})
	h.expect(http.StatusForbidden, "ann", "POST", "/workflow-runs/"+f.run+"/tasks/"+f.issue+"/link", nil)
	h.expect(http.StatusForbidden, "ann", "PUT", "/cases/"+f.kase+"/tasks/"+f.issue, nil)
	h.expect(http.StatusForbidden, "ann", "POST", "/workflow-runs/"+f.run+"/cancel", map[string]any{"reason": "no", "quiesce_deadline": deadline})
	h.expect(http.StatusForbidden, "ann", "POST", "/workflow-runs/"+f.run+"/rollback", map[string]any{"reason": "no"})
	h.expect(http.StatusForbidden, "ann", "POST", "/cases/"+f.kase+"/end", map[string]any{"status": "cancelled"})
}

func TestCanSeeDoesNotAllowTheOtherWritesOnAnIssue(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.refusedForAnn(t)
	got := f.h.expect(http.StatusOK, "boss", "GET", "/tasks/"+f.issue, nil)
	if got["status"] != "pending" || got["assigned_to"] != nil && got["assigned_to"] != "" {
		t.Fatalf("a refused write changed the issue: %v", got)
	}
}

func TestCanMoveAllowsTheOtherWritesOnAnIssue(t *testing.T) {
	f := newMoreWritesFixture(t)
	f.h.expect(http.StatusOK, "boss", "PUT", "/backlogs/"+f.docs+"/access/contributors", map[string]string{"level": "move"})
	f.h.expect(http.StatusNoContent, "ann", "POST", "/tasks/"+f.issue+"/unassign", nil)
	f.h.expect(http.StatusNoContent, "ann", "POST", "/projects/"+f.project+"/tasks/"+f.issue, nil)
	f.h.expect(http.StatusNoContent, "ann", "DELETE", "/projects/"+f.project+"/tasks/"+f.issue, nil)
	f.h.expect(http.StatusCreated, "ann", "POST", "/projects/guide/tasks", map[string]any{"title": "new", "backlog": f.docs, "step": "write"})
}
