package routes

import (
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// ProjectRoutes returns the plain Project CRUD and task-project linking
// routes. GET /projects with a `?name=` query param resolves
// GetProjectByName (a single object) instead of listing — a separate
// /by-name/{name} path collides with /{projectID}/tasks under net/http's
// ServeMux (both are two-segment wildcard patterns with the same shape).
func ProjectRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return []Route{
		{Method: "POST", Path: "/projects", Handler: CreateProject(tm)},
		{Method: "GET", Path: "/projects", Handler: ListProjects(tm)},
		{Method: "GET", Path: "/projects/{projectID}", Handler: GetProject(tm)},
		{Method: "PUT", Path: "/projects/{projectID}", Handler: UpdateProject(tm)},
		{Method: "DELETE", Path: "/projects/{projectID}", Handler: DeleteProject(tm)},
		{Method: "GET", Path: "/projects/{projectID}/tasks", Handler: ListTasksInProject(tm)},
		{Method: "POST", Path: "/projects/{projectID}/tasks/{taskID}", Handler: AddTaskToProject(tm)},
		{Method: "DELETE", Path: "/projects/{projectID}/tasks/{taskID}", Handler: RemoveTaskFromProject(tm)},
		{Method: "GET", Path: "/tasks/{taskID}/projects", Handler: ListProjectsForTask(tm)},
	}
}

func CreateProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p mwanachamataskmanager.Project
		if err := readJSON(r, &p); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out, err := tm.CreateProject(r.Context(), p)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

func ListProjects(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if name := r.URL.Query().Get("name"); name != "" {
			p, err := tm.GetProjectByName(r.Context(), name)
			if err != nil {
				writeTaskErr(w, err)
				return
			}
			writeJSON(w, http.StatusOK, p)
			return
		}
		projects, err := tm.ListProjects(r.Context())
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, projects)
	}
}

func GetProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := tm.GetProject(r.Context(), r.PathValue("projectID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func UpdateProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var p mwanachamataskmanager.Project
		if err := readJSON(r, &p); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		p.ID = r.PathValue("projectID")
		out, err := tm.UpdateProject(r.Context(), p)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func DeleteProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.DeleteProject(r.Context(), r.PathValue("projectID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func AddTaskToProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tm.AddTaskToProject(r.Context(), r.PathValue("taskID"), r.PathValue("projectID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func RemoveTaskFromProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tm.RemoveTaskFromProject(r.Context(), r.PathValue("taskID"), r.PathValue("projectID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func ListTasksInProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tasks, err := tm.ListTasksInProject(r.Context(), r.PathValue("projectID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	}
}

func ListProjectsForTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projects, err := tm.ListProjectsForTask(r.Context(), r.PathValue("taskID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, projects)
	}
}
