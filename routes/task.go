package routes

import (
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// TaskRoutes returns the plain Task CRUD routes.
func TaskRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return []Route{
		{Method: "POST", Path: "/tasks", Handler: CreateTask(tm)},
		{Method: "GET", Path: "/tasks", Handler: ListTasks(tm)},
		{Method: "GET", Path: "/tasks/{taskID}", Handler: GetTask(tm)},
		{Method: "PUT", Path: "/tasks/{taskID}", Handler: UpdateTask(tm)},
		{Method: "DELETE", Path: "/tasks/{taskID}", Handler: DeleteTask(tm)},
		{Method: "POST", Path: "/tasks/{taskID}/unblock", Handler: UnblockTask(tm)},
		{Method: "GET", Path: "/projects/{projectName}/tasks/by-name/{taskName}", Handler: GetTaskByName(tm)},
		{Method: "POST", Path: "/projects/{projectName}/tasks", Handler: CreateTaskInProject(tm)},
	}
}

func CreateTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var task mwanachamataskmanager.Task
		if err := readJSON(r, &task); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out, err := tm.CreateTask(r.Context(), task)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

func ListTasks(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		filter := mwanachamataskmanager.TaskFilter{
			Status:        mwanachamataskmanager.TaskStatus(q.Get("status")),
			Priority:      mwanachamataskmanager.TaskPriority(q.Get("priority")),
			WorkflowRunID: q.Get("workflow_run_id"),
		}
		tasks, err := tm.ListTasks(r.Context(), filter)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tasks)
	}
}

func GetTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, err := tm.GetTask(r.Context(), r.PathValue("taskID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func UpdateTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var task mwanachamataskmanager.Task
		if err := readJSON(r, &task); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		task.ID = r.PathValue("taskID")
		out, err := tm.UpdateTask(r.Context(), task)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func DeleteTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.DeleteTask(r.Context(), r.PathValue("taskID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func UnblockTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Note string `json:"note"`
		}
		if r.ContentLength > 0 {
			if err := readJSON(r, &body); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid request body")
				return
			}
		}
		task, err := tm.UnblockTask(r.Context(), r.PathValue("taskID"), body.Note)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func GetTaskByName(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		task, err := tm.GetTaskByName(r.Context(), r.PathValue("projectName"), r.PathValue("taskName"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, task)
	}
}

func CreateTaskInProject(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var task mwanachamataskmanager.Task
		if err := readJSON(r, &task); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out, err := tm.CreateTaskInProject(r.Context(), r.PathValue("projectName"), task)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}
