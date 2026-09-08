package routes

import (
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// WorkflowRunRoutes returns the WorkflowRun creation, linking, closure
// read, status, and failure-budget routes. Cancel/rollback/watchdog live in
// [WorkflowRunLifecycleRoutes].
func WorkflowRunRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return []Route{
		{Method: "POST", Path: "/workflow-runs", Handler: CreateWorkflowRun(tm)},
		{Method: "POST", Path: "/workflow-runs/recovery", Handler: CreateRecoveryWorkflowRun(tm)},
		// GET /workflow-runs?name=... resolves GetWorkflowRunByName (a single
		// object) instead of listing — a separate /by-name/{name} path
		// collides with /{runID}/closure and /{runID}/tasks under net/http's
		// ServeMux.
		{Method: "GET", Path: "/workflow-runs", Handler: ListWorkflowRuns(tm)},
		{Method: "GET", Path: "/workflow-runs/{runID}", Handler: GetWorkflowRun(tm)},
		{Method: "PUT", Path: "/workflow-runs/{runID}/status", Handler: UpdateWorkflowRunStatus(tm)},
		{Method: "GET", Path: "/workflow-runs/{runID}/closure", Handler: GetWorkflowRunClosure(tm)},
		{Method: "GET", Path: "/workflow-runs/{runID}/tasks", Handler: ListTasksForRun(tm)},
		{Method: "POST", Path: "/workflow-runs/{runID}/tasks/{taskID}/link", Handler: LinkTaskToRun(tm)},
		{Method: "POST", Path: "/workflow-runs/{runID}/todos/{todoID}/link", Handler: LinkTodoToRun(tm)},
		{Method: "PUT", Path: "/workflow-runs/{runID}/failure-budget", Handler: SetFailureBudget(tm)},
		{Method: "POST", Path: "/workflow-runs/{rootRunID}/failure-budget/increment", Handler: IncrementFailureBudget(tm)},
	}
}

func CreateWorkflowRun(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name         string `json:"name"`
			TriggerEvent string `json:"trigger_event,omitempty"`
			Initiator    string `json:"initiator,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		run, err := tm.CreateWorkflowRun(r.Context(), body.Name, body.TriggerEvent, body.Initiator)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, run)
	}
}

func CreateRecoveryWorkflowRun(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name         string `json:"name"`
			TriggerEvent string `json:"trigger_event,omitempty"`
			Initiator    string `json:"initiator,omitempty"`
			ParentRunID  string `json:"parent_run_id"`
			RootRunID    string `json:"root_run_id,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		run, err := tm.CreateRecoveryWorkflowRun(r.Context(),
			body.Name, body.TriggerEvent, body.Initiator, body.ParentRunID, body.RootRunID)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, run)
	}
}

func GetWorkflowRun(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		run, err := tm.GetWorkflowRun(r.Context(), r.PathValue("runID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

func ListWorkflowRuns(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if name := r.URL.Query().Get("name"); name != "" {
			run, err := tm.GetWorkflowRunByName(r.Context(), name)
			if err != nil {
				writeTaskErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, run)
			return
		}
		runs, err := tm.ListWorkflowRuns(r.Context(), "")
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

func LinkTaskToRun(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tm.LinkTaskToRun(r.Context(), r.PathValue("runID"), r.PathValue("taskID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func LinkTodoToRun(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tm.LinkTodoToRun(r.Context(), r.PathValue("runID"), r.PathValue("todoID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// workflowRunClosureJSON is [mwanachamataskmanager.WorkflowRunClosure] with
// its untagged Edges field converted via relationshipJSON.
type workflowRunClosureJSON struct {
	Run            mwanachamataskmanager.WorkflowRun `json:"run"`
	Tasks          []mwanachamataskmanager.Task      `json:"tasks"`
	Todos          []mwanachamataskmanager.TaskTodo  `json:"todos"`
	Edges          []relationshipJSON                `json:"edges"`
	AgentRunIDs    []string                          `json:"agent_run_ids,omitempty"`
	FunctionJobIDs []string                          `json:"function_job_ids,omitempty"`
	BranchNames    []string                          `json:"branch_names,omitempty"`
}

func GetWorkflowRunClosure(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		closure, err := tm.GetWorkflowRunClosure(r.Context(), r.PathValue("runID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		edges := make([]relationshipJSON, len(closure.Edges))
		for i, e := range closure.Edges {
			edges[i] = toRelationshipJSON(e)
		}
		writeJSON(w, http.StatusOK, workflowRunClosureJSON{
			Run: closure.Run, Tasks: closure.Tasks, Todos: closure.Todos, Edges: edges,
			AgentRunIDs: closure.AgentRunIDs, FunctionJobIDs: closure.FunctionJobIDs,
			BranchNames: closure.BranchNames,
		})
	}
}

func UpdateWorkflowRunStatus(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			NewStatus mwanachamataskmanager.WorkflowRunStatus `json:"new_status"`
			Reason    string                                  `json:"reason,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		run, err := tm.UpdateWorkflowRunStatus(r.Context(), r.PathValue("runID"), body.NewStatus, body.Reason)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

func ListTasksForRun(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := tm.ListTasksForRun(r.Context(), r.PathValue("runID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	}
}

func SetFailureBudget(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Budget int `json:"budget"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		run, err := tm.SetFailureBudget(r.Context(), r.PathValue("runID"), body.Budget)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

func IncrementFailureBudget(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChildRunID string `json:"child_run_id"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		used, budget, exhausted, err := tm.IncrementFailureBudget(r.Context(), r.PathValue("rootRunID"), body.ChildRunID)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"used": used, "budget": budget, "exhausted": exhausted,
		})
	}
}
