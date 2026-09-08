package routes

import (
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// AssignmentRoutes returns the Task assignment and generic relationship
// edge routes.
func AssignmentRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return []Route{
		{Method: "POST", Path: "/tasks/{taskID}/assign", Handler: AssignTask(tm)},
		{Method: "POST", Path: "/tasks/{taskID}/unassign", Handler: UnassignTask(tm)},
		{Method: "POST", Path: "/tasks/{taskID}/unblock-dependents", Handler: UnblockDependents(tm)},
		{Method: "POST", Path: "/relationships", Handler: CreateRelationship(tm)},
		{Method: "DELETE", Path: "/relationships", Handler: DeleteRelationship(tm)},
		{Method: "GET", Path: "/relationships/traverse", Handler: TraverseRelationships(tm)},
	}
}

func AssignTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			AgentID       string `json:"agent_id"`
			WorkflowRunID string `json:"workflow_run_id"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		err := tm.AssignTask(r.Context(), r.PathValue("taskID"), body.AgentID, body.WorkflowRunID)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func UnassignTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.UnassignTask(r.Context(), r.PathValue("taskID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func UnblockDependents(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.UnblockDependents(r.Context(), r.PathValue("taskID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func CreateRelationship(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Label      string         `json:"label"`
			FromID     string         `json:"from_id"`
			ToID       string         `json:"to_id"`
			Properties map[string]any `json:"properties,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		rel, err := tm.CreateRelationship(r.Context(), mwanachamataskmanager.Relationship{
			Label: body.Label, FromID: body.FromID, ToID: body.ToID, Properties: body.Properties,
		})
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toRelationshipJSON(rel))
	}
}

func DeleteRelationship(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			FromID string `json:"from_id"`
			ToID   string `json:"to_id"`
			Label  string `json:"label"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		err := tm.DeleteRelationship(r.Context(), body.FromID, body.ToID, body.Label)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func TraverseRelationships(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		dir := mwanachamataskmanager.DirectionOutbound
		if q.Get("direction") == "inbound" {
			dir = mwanachamataskmanager.DirectionInbound
		}
		rels, err := tm.TraverseRelationships(r.Context(), q.Get("vertex_id"), q.Get("label"), dir)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		out := make([]relationshipJSON, len(rels))
		for i, rel := range rels {
			out[i] = toRelationshipJSON(rel)
		}
		writeJSON(w, http.StatusOK, out)
	}
}
