// importroutes.go — named to avoid colliding with the stdlib "import"
// keyword as a file concept; the package itself is unaffected.
package routes

import (
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// ImportRoutes returns the async project-import routes.
func ImportRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return []Route{
		{Method: "POST", Path: "/projects/import", Handler: ImportProject(tm)},
		{Method: "POST", Path: "/projects/import/jobs", Handler: StartImportProject(tm)},
		{Method: "GET", Path: "/projects/import/jobs/{jobID}", Handler: GetImportProjectStatus(tm)},
		{Method: "POST", Path: "/projects/import/jobs/{jobID}/cancel", Handler: CancelImportProject(tm)},
	}
}

// importResultJSON is [mwanachamataskmanager.ImportResult] with JSON tags —
// that struct carries none of its own.
type importResultJSON struct {
	Project      mwanachamataskmanager.Project `json:"project"`
	Tasks        []mwanachamataskmanager.Task  `json:"tasks"`
	DepsCreated  int                           `json:"deps_created"`
	TasksCreated int                           `json:"tasks_created"`
}

func toImportResultJSON(r mwanachamataskmanager.ImportResult) importResultJSON {
	return importResultJSON{
		Project: r.Project, Tasks: r.Tasks,
		DepsCreated: r.DepsCreated, TasksCreated: r.TasksCreated,
	}
}

func ImportProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Document string `json:"document"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		result, err := tm.ImportProject(r.Context(), body.Document)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, toImportResultJSON(result))
	}
}

func StartImportProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Document string `json:"document"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		job, err := tm.StartImportProject(r.Context(), body.Document)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	}
}

func GetImportProjectStatus(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		job, err := tm.GetImportProjectStatus(r.Context(), r.PathValue("jobID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
	}
}

func CancelImportProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.CancelImportProject(r.Context(), r.PathValue("jobID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
