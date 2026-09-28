# Routes

An address is an entry in `taskmanager.operations.json`, and adding one is an
edit to that file rather than a new Go function. `shared/dispatch` reads the
declaration, finds the manager method it names, checks their signatures
against each other and builds the handler. `routes/routes.go` is the adapter:
the sentinel table, the anonymous allowlist, and the four constructor shapes
every spec-driven module has.

Sixty operations are declared. `TestEveryDeclaredOperationMatchesTheManager`
builds them all against a real manager, so a declaration that drifts from its
method stops the process starting rather than failing one request.

## Nothing is anonymous

`AnonymousActions` is empty on purpose. A work board has no public half the
way a storefront catalog does, so every route arrives gated and
`TestNoRouteIsAnonymous` holds it that way. The allowlist stays rather than
being removed, so an operation added to the spec and not named there is
gated by default.

This module has no auth model of its own, matching
`mwanachama-backend-catalog` and `mwanachama-backend-assetmanager`: a route
still needs a capability gate wrapped around it by whatever mounts it. The
action ids are what a mount names — `taskmanager.task.create`,
`taskmanager.workflow_run.rollback` — never this module's Go identifiers.

## Mounting per instance

`Shape()` returns the declared table with no handlers, for a mount that
resolves its manager per request instead of holding one. Its order is
`Dispatch`'s own, so `Shape()[i]` and `Routes(tm)[i]` are the same operation.
`mwanachama-wakala-api` mounts this way, under
`/instances/{instanceID}/taskmanager`.

## Addresses that changed

The declared table is the same one the hand-written handlers served, with
four exceptions worth knowing about.

**The two watchdog sweeps became POST.** `GET /workflow-runs/stale?cutoff=…`
and `GET /workflow-runs/step-stale?cutoff=…` are now `POST` with
`{"cutoff": "…"}` in the body. The dispatcher binds a non-string argument by
`json.Unmarshal`, and a bare RFC 3339 timestamp in a query string is not
valid JSON — it needs the quotes a body gives it. Both are operator sweeps
run by the watchdog, not browser reads, so the method costs nothing. The
alternative is teaching `shared/dispatch` to parse a timestamp from text,
which would change the engine catalog, agency and workspace all run on;
that is a proposal for the engine, not something to do from here.

**A refusal carries no extra fields.** The old `writeTaskErr` answered a
blocked transition with `{"error": …, "blocker_task_ids": [...]}`. The
dispatcher writes `{"error": …}` for every refusal, so the blocker ids are
gone from the response body. `ErrBlocked` still maps to 409, and the ids are
still reachable — `GET /relationships/traverse?vertex_id={taskID}&label=blocks&direction=inbound`
answers the same question. Restoring the field would mean a per-operation
error renderer in the shared dispatcher.

**Two response shapes come from the model, not a hand-written mirror.**
`Relationship` and `ImportResult` were marshalled through `relationshipJSON`
and `importResultJSON` wrappers that existed only to add snake_case tags.
Both types now carry those tags themselves, so the wire shape is unchanged
and the wrappers are gone.

**`WorkflowRunClosure` already carried its own tags** and needed nothing.

## Status codes

Taken from the handlers they replace: `201` on the eight creates, `202` on
`POST /projects/import/jobs`, `204` on everything that returns only an error,
`200` otherwise.

## The error table

`taskmanager.operations.json`'s `errors` block maps each sentinel name to a
status, and `routes/routes.go`'s `sentinels` map turns those names into the
package's real error values. The two are the same list `writeTaskErr` held in
one `switch`, and splitting them this way is what lets a mount read the
status a refusal will carry without importing the handler.
