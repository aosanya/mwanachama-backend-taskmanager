package routes

import (
	"net/http"
	"time"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// WorkflowRunLifecycleRoutes returns the WorkflowRun rollback, cancellation,
// and watchdog sweep read/write routes.
func WorkflowRunLifecycleRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return []Route{
		{Method: "POST", Path: "/workflow-runs/{runID}/rollback", Handler: RollbackWorkflowRun(tm)},
		{Method: "DELETE", Path: "/workflow-runs/{runID}/artifacts", Handler: DeleteWorkflowRunArtifacts(tm)},
		{Method: "POST", Path: "/workflow-runs/{runID}/cancel", Handler: CancelWorkflowRun(tm)},
		{Method: "POST", Path: "/workflow-runs/{runID}/finalize-cancel", Handler: FinalizeWorkflowRunCancellation(tm)},
		{Method: "PUT", Path: "/workflow-runs/{runID}/last-event-at", Handler: TouchWorkflowRunLastEventAt(tm)},
		{Method: "GET", Path: "/workflow-runs/stale", Handler: ListWorkflowRunsStaleSince(tm)},
		{Method: "GET", Path: "/workflow-runs/step-stale", Handler: ListWorkflowRunsStepStaleSince(tm)},
		{Method: "POST", Path: "/workflow-runs/{runID}/mark-timeout-published", Handler: MarkTimeoutPublished(tm)},
		{Method: "POST", Path: "/workflow-runs/{runID}/handle-timeout", Handler: HandleRunTimeout(tm)},
		{Method: "POST", Path: "/workflow-runs/{runID}/tasks/{taskOrTodoID}/handle-timeout", Handler: HandleTaskTimeout(tm)},
	}
}

func RollbackWorkflowRun(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Reason string `json:"reason,omitempty"`
		}
		if r.ContentLength > 0 {
			if err := readJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid request body")
				return
			}
		}
		run, err := tm.RollbackWorkflowRun(r.Context(), r.PathValue("runID"), body.Reason)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

func DeleteWorkflowRunArtifacts(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.DeleteWorkflowRunArtifacts(r.Context(), r.PathValue("runID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func CancelWorkflowRun(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Reason          string `json:"reason,omitempty"`
			CancelledBy     string `json:"cancelled_by,omitempty"`
			QuiesceDeadline string `json:"quiesce_deadline,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		deadline := time.Now().UTC()
		if body.QuiesceDeadline != "" {
			parsed, err := time.Parse(time.RFC3339, body.QuiesceDeadline)
			if err != nil {
				writeErr(w, http.StatusBadRequest, "quiesce_deadline must be RFC 3339")
				return
			}
			deadline = parsed
		}
		run, err := tm.CancelWorkflowRun(r.Context(), r.PathValue("runID"), body.Reason, body.CancelledBy, deadline)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

func FinalizeWorkflowRunCancellation(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, err := tm.FinalizeWorkflowRunCancellation(r.Context(), r.PathValue("runID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

func TouchWorkflowRunLastEventAt(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Timestamp string `json:"timestamp"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		err := tm.TouchWorkflowRunLastEventAt(r.Context(), r.PathValue("runID"), body.Timestamp)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func parseCutoff(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
	v := r.URL.Query().Get("cutoff")
	if v == "" {
		writeErr(w, http.StatusBadRequest, "cutoff is required (RFC 3339)")
		return time.Time{}, false
	}
	cutoff, err := time.Parse(time.RFC3339, v)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "cutoff must be RFC 3339")
		return time.Time{}, false
	}
	return cutoff, true
}

func ListWorkflowRunsStaleSince(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cutoff, ok := parseCutoff(w, r)
		if !ok {
			return
		}
		runs, err := tm.ListWorkflowRunsStaleSince(r.Context(), cutoff)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

func ListWorkflowRunsStepStaleSince(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cutoff, ok := parseCutoff(w, r)
		if !ok {
			return
		}
		runs, err := tm.ListWorkflowRunsStepStaleSince(r.Context(), cutoff)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

func MarkTimeoutPublished(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.MarkTimeoutPublished(r.Context(), r.PathValue("runID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func HandleRunTimeout(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.HandleRunTimeout(r.Context(), r.PathValue("runID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func HandleTaskTimeout(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tm.HandleTaskTimeout(r.Context(), r.PathValue("taskOrTodoID"), r.PathValue("runID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
