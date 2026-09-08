// Package routes is mwanachama-backend-taskmanager's own HTTP surface:
// decode a request, call one [mwanachamataskmanager.TaskManager] method,
// encode the response — the same shape the root package's Go callers
// already get, just reachable from an HTTP mux. It exists so a route's
// request/response shape and its domain logic are authored and reviewed
// together, in the package that owns the domain, rather than reimplemented
// a second time in whichever process happens to mount this package.
//
// Ported from mwanachama-backend-api-gateway's
// internal/api/http/taskmanager_handlers*.go — every one of that package's
// 60 routes is, underneath, a single TaskManager call with no gateway-only
// policy layered on top (unlike mwanachama-backend-actor's routes package,
// which excludes ten operations that compose with gateway-only domains),
// so the entire set moved here unchanged in shape: [Routes] returns all of
// them as one list; [TaskRoutes]/[AgentRoutes]/[ProjectRoutes]/
// [AssignmentRoutes]/[DeliverableRoutes]/[ImportRoutes]/
// [WorkflowRunRoutes]/[WorkflowRunLifecycleRoutes] return one file's worth
// at a time, for a mounting process that wraps different groups in
// different policy.
//
// A route built from this package still needs a caller-identity/capability
// gate wrapped around it before it is safe to serve — this package answers
// "what happens once that gate has passed", never "who may pass it". The
// mounting process supplies that gate by wrapping the http.HandlerFunc this
// package returns (mux.HandleFunc(rt.Pattern(prefix), d.auth(d.operator(Cap,
// rt.Handler)))), not by this package reaching for a session or a
// capability itself.
package routes
