package routes

import (
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// DeliverableRoutes returns the Deliverable/AcceptanceCriteria reads and
// TaskTodo CRUD routes.
func DeliverableRoutes(tm mwanachamataskmanager.TaskManager) []Route {
	return []Route{
		{Method: "GET", Path: "/tasks/{taskID}/deliverables", Handler: ListDeliverablesForTask(tm)},
		{Method: "GET", Path: "/tasks/{taskID}/acceptance-criteria", Handler: ListAcceptanceCriteriaForTask(tm)},
		{Method: "PUT", Path: "/acceptance-criteria/{criteriaID}/result", Handler: WriteAcceptanceCriteriaResult(tm)},
		{Method: "POST", Path: "/todos", Handler: CreateTaskTodo(tm)},
		{Method: "GET", Path: "/todos/{todoID}", Handler: GetTaskTodo(tm)},
		{Method: "POST", Path: "/todos/{todoID}/dispatch", Handler: DispatchTaskTodo(tm)},
		{Method: "PUT", Path: "/todos/{todoID}/status", Handler: UpdateTaskTodoStatus(tm)},
		{Method: "GET", Path: "/workflow-runs/{runID}/todos", Handler: ListTaskTodos(tm)},
	}
}

func ListDeliverablesForTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deliverables, err := tm.ListDeliverablesForTask(r.Context(), r.PathValue("taskID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, deliverables)
	}
}

func ListAcceptanceCriteriaForTask(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		criteria, err := tm.ListAcceptanceCriteriaForTask(r.Context(), r.PathValue("taskID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, criteria)
	}
}

func WriteAcceptanceCriteriaResult(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Result string `json:"result"`
			Notes  string `json:"notes,omitempty"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		err := tm.WriteAcceptanceCriteriaResult(r.Context(), r.PathValue("criteriaID"), body.Result, body.Notes)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func CreateTaskTodo(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var todo mwanachamataskmanager.TaskTodo
		if err := readJSON(r, &todo); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		out, err := tm.CreateTaskTodo(r.Context(), todo)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, out)
	}
}

func GetTaskTodo(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		todo, err := tm.GetTaskTodo(r.Context(), r.PathValue("todoID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, todo)
	}
}

func DispatchTaskTodo(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := tm.DispatchTaskTodo(r.Context(), r.PathValue("todoID")); err != nil {
			writeTaskErr(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func UpdateTaskTodoStatus(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Status mwanachamataskmanager.TodoStatus `json:"status"`
		}
		if err := readJSON(r, &body); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid request body")
			return
		}
		todo, err := tm.UpdateTaskTodoStatus(r.Context(), r.PathValue("todoID"), body.Status)
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, todo)
	}
}

func ListTaskTodos(tm mwanachamataskmanager.TaskManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		todos, err := tm.ListTaskTodos(r.Context(), r.PathValue("runID"))
		if err != nil {
			writeTaskErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, todos)
	}
}
