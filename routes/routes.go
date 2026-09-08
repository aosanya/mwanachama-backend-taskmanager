// Package routes provides HTTP handlers over [mwanachamataskmanager.TaskManager],
// mounted by mwanachama-backend-api-gateway. See doc.go for scope.
package routes

import (
	"net/http"

	mwanachamataskmanager "github.com/aosanya/mwanachama-backend-taskmanager"
)

// Route is one HTTP endpoint: a method, a path relative to this package's
// mount point, and the handler. The mounting process wraps Handler with its
// own auth/capability gates and builds the mux itself — see doc.go.
type Route struct {
	Method  string
	Path    string
	Handler http.HandlerFunc
}

// Pattern returns the Go 1.22+ ServeMux pattern for this route under
// prefix, e.g. Pattern("/v1/taskmanager") on {Method: "GET", Path:
// "/tasks/{taskID}"} yields "GET /v1/taskmanager/tasks/{taskID}".
func (rt Route) Pattern(prefix string) string {
	return rt.Method + " " + prefix + rt.Path
}

// Routes returns every route this package defines, over tm. Concatenates
// the per-entity route lists — see task.go, agent.go, project.go,
// assignment.go, deliverable.go, importroutes.go, workflowrun.go, and
// workflowrun_lifecycle.go.
func Routes(tm mwanachamataskmanager.TaskManager) []Route {
	var out []Route
	out = append(out, TaskRoutes(tm)...)
	out = append(out, AgentRoutes(tm)...)
	out = append(out, ProjectRoutes(tm)...)
	out = append(out, AssignmentRoutes(tm)...)
	out = append(out, DeliverableRoutes(tm)...)
	out = append(out, ImportRoutes(tm)...)
	out = append(out, WorkflowRunRoutes(tm)...)
	out = append(out, WorkflowRunLifecycleRoutes(tm)...)
	return out
}
