# CLAUDE.md

Guidance for Claude Code working in this repository.

## Project: mwanachama-backend-taskmanager

Postgres port of `CodeValdWork` for [mwanachama-frontend-kazi](../mwanachama-frontend-kazi).
Module path `github.com/aosanya/mwanachama-backend-taskmanager`.

Converted to the spec-driven shape on 2026-09-28 — `mwanachama-backend-catalog`'s
format, on `mwanachama-backend-shared`'s `spec`/`specstore`/`dispatch`
engine. See [documentation/2. design/declared-objects.md](documentation/2.%20design/declared-objects.md)
and [documentation/2. design/routes.md](documentation/2.%20design/routes.md).

Dropped from the original: `proto/`, `cmd/server`, `internal/server` (gRPC
`TaskServiceServer` + the ~1150-line `TaskEventDispatcher` that drives
AI-failure-recovery orchestration off inbound events), `internal/registrar`
(CodeValdCortex Cross heartbeat). `mwanachama-backend-api-gateway` runs as one
service and imports this package directly — there is no separate process to
dispatch events to. If the escalation/retry orchestration the dispatcher did
is still needed, it has to be re-homed as in-process logic here or in the
gateway, not assumed to exist.

## Objects are declared, not written

The tables come from a JSON spec, not from Go structs.

- `taskmanager.blueprint.json` — **the module's fourteen objects, declared
  once**. Embedded and reached through `Blueprint()`, `LoadSpec(path)` and
  `ParseSpec(raw)`. Load a domain spec through those, never through
  `spec.Load`, or its roled objects arrive with no fields.
- `spec/examples/work.taskmanager.json` and `agency.taskmanager.json` — the
  same module under two domains. `TestShippedExamplesCoexist` migrates both
  into one database. The agency one is also the shipped domain: `SpecFor`
  embeds it and swaps the instance.
- `taskmanager.operations.json` — the route table, built by `dispatch`.

**A domain names objects; it does not re-declare them.** Its spec supplies
`instance`, the name and table each role lands in, its own indexes, and a
default on a declared field — nothing else. Anything more is refused by name
at load.

**Adding a field means editing the blueprint and the Go type together.**
`NewTaskManager` checks the spec and `carriers()` against each other and
refuses to build if a declared column has no field to hold it, or a field no
column to land in. `TestEveryExampleFitsTheTypes` runs that agreement against
*every* spec under `spec/examples/`, not just the one a test happened to
load.

**A table is `<instance>_<module>_<object>`.** The module segment is not
decoration: the agency module also declares a `work_item`, and without the
segment a taskmanager instance named `wakala` and an agency instance of the
same name both want `wakala_work_items` — a silent collision, not an error,
because `AutoMigrate` adopts a table that already exists.

**A second mount of this module in one instance adds a fourth segment** —
`<instance>_taskmanager_<mount>_<object>`, via `SpecForMount(instance,
mount)`; `SpecFor` is the default mount, whose segment is elided, so its
names are unchanged. ⚠️ **This module has effectively no room for it.**
`<slug11>_taskmanager_workflow_runs_parent_workflow_run_idx` is 61 of 63
bytes against the real 11-character `tableSlug()`, leaving room for a mount
name of **one character** — so in practice taskmanager is single-mount until
`mwanachama-backend-shared`'s S28 shortens how index names are built. The
short instance names this repo's own tests use (`wakala`, `kazi`) hide this:
`SpecForMount("wakala", "second")` passes at 6 characters because `wakala` is
5 bytes shorter than a real slug. Measure against a real slug, not a fixture.

**A column is found by field name, never by json tag** —
`specstore.ColumnName` turns `SubmittedBy` into `submitted_by`. **Every
declared column is written on every write**, because a map missing a key
means "leave it alone" to an update.

**A stored enum value outlives a rename**, so
`TestVocabularyMatchesTheBlueprint` holds the constants Go compares against
and the blueprint's declared `values` to each other in both directions.

## Routes are declared too

An address is an entry in `taskmanager.operations.json`; adding one is an
edit to that file rather than a new Go function.
[[feedback_routes_name_match_model]] still governs any repo that has not
adopted the dispatcher.

`AnonymousActions` is empty on purpose — a work board has no public half, so
every route arrives gated and `TestNoRouteIsAnonymous` holds it there. This
module has **no auth model**: a route still needs a capability gate wrapped
around it by whatever mounts it.

`Shape()` is the declared table with no handlers, in `Dispatch`'s own order,
for a mount that resolves its manager per request —
`mwanachama-wakala-api` gives each registered agency its own board that way.

## What is superseded

- **`gormstore/`** — deleted 2026-09-28. Root `tables.go` and its
  `TableNames`/`DefaultTableNames` went with it;
  `NewTaskManager(db, *spec.Spec, pub)` is the constructor now. No other
  repo imported this one, so nothing needed a shim.
- **`routes/`'s thirteen hand-written route builders** — deleted the same
  day, along with `writeTaskErr` and the `relationshipJSON`/
  `importResultJSON` wrappers.
- Four addresses changed as a consequence. They are listed in
  [routes.md](documentation/2.%20design/routes.md) — read it before assuming
  an address is what it was.

## Behaviours to preserve

Closed 2026-09-28 (W14, W15, W19–W22); each has a mutation-checked test, and
the reasoning is in
[documentation/2. design/concurrency-and-sequence.md](documentation/2.%20design/concurrency-and-sequence.md).

- **A write re-checks what its read assumed.** `UpdateTask`,
  `UpdateProject` and `UpdateTaskTodoStatus` all carry
  `AND deleted = false` in the write's own `WHERE` and report not-found on
  0 rows affected. Reading through a getter that filters deleted rows is not
  enough — the delete lands in the gap.
- **The code counter is advanced by the database**, never read-modify-written
  in Go: insert-on-conflict-do-nothing, `next_number = next_number + 1`, then
  read back what this caller claimed. Writing first is what stops SQLite
  deadlocking on a read-to-write upgrade.
- **The failure budget is charged under a compare-and-swap** on the counter
  the call read, retried on a loss. Charging one child twice stays a no-op.
- **A rollback has one entry point.** Compensation
  (`DeleteWorkflowRunArtifacts`) requires the run to be in `rolling_back`;
  the three rollback states are refused by the public status setter and
  reached only through `RollbackWorkflowRun`'s unexported `setRunStatus`.
  `CanTransitionTo` is untouched on purpose — it says which transitions are
  legal, not who may ask for one.

A race test that passes both before and after a fix is not a regression
guard: W19's and W20's original pins counted a legitimate
update-then-delete ordering alongside the defect. All three soft-delete
guards force the interleave with a GORM callback instead of racing for it.

## Porting notes

- `task.go`'s `TaskManager` interface and `models.go`'s domain types (Task,
  Agent, Project, WorkflowRun, Deliverable, AcceptanceCriteria, Tag,
  TaskTodo) port unchanged, **including** the hand-rolled `CanTransitionTo`
  state machines (Task: 7 states incl. `blocked`/`awaiting-direction`/
  `split`; WorkflowRun: 9 states incl. `paused`/`cancelling`/`rolling_back`/
  `rollback_failed`) — these are pure Go, no storage dependency.
- `schema.go`'s `DefaultWorkSchema()` is now `taskmanager.blueprint.json`;
  vertex uniqueness (Agent by `agent_id`, Tag by `name`) is a declared
  `unique` index.
- Straightforward CRUD/business-logic files (task, converters, project,
  assignment(+unblock), deliverable, relationship engine, todo, agent) only
  ever call `entitygraph.DataManager` — port with minimal churn.
- **`WorkflowRun` is the hard part.** Closure queries
  (`GetWorkflowRunClosure`), cascade cancel, failure-budget accounting, and
  the watchdog all currently lean on Arango's traversal; rebuild them as
  recursive CTEs. `RollbackWorkflowRun`'s 4-step compensation sequence
  (transition → per-service compensation → hard-delete artifacts → terminal
  transition) needs to run inside a real `pgx.Tx` — the Arango original had
  no equivalent atomicity guarantee, so this is new transactional design, not
  a mechanical port. Guard against `ErrForeignRunDependency` (cross-run
  dependency violations) the same way the original does.
- `import.go` ("ImportProject") means project-bootstrap-from-JSON, not a Go
  import — ports as-is.

## Conventions

- `go test ./...` (sqlite via `glebarez/sqlite`) is the expected way to
  verify a change here — do not reach for a real Postgres. See
  [[feedback_use_memory_backend_for_tests]]. `postgres_integration_test.go`
  is `//go:build integration`, gated on `POSTGRES_URL`, and not part of it.
- No Go file over 300 lines; split by responsibility.
- Task status lives on
  [documentation/3. implementation/todo.md](documentation/3.%20implementation/todo.md).
- Four-phase `documentation/` layout — see
  [documentation/README.md](documentation/README.md).
- Before wiring into `mwanachama-backend-api-gateway`, check
  `internal/domain/agentic` there for naming/scope overlap.

## Code comments

Write code with no comments. Not one-liners above a function, not section
banners, not doc comments on exported symbols, not "why" notes next to a
tricky line. A name, a type, or a smaller function carries it instead.

Anything that genuinely needs explaining goes in this repo's `documentation/`
folder, under the phase it belongs to (`1. requirements`, `2. design`,
`3. implementation`, `4. qa`) — never inline.

**Why:** inline prose drifts out of sync with the code, duplicates what
`documentation/` already owns, and buries the explanation where nobody
looking for it will search.

**How to apply:**

- New code ships without comments. If a line seems to need one, rename or
  split until it doesn't.
- Touching code that already has comments: strip the ones in the code you are
  changing. Do not sweep untouched files unless asked.
- If the reasoning matters, add or update the matching `documentation/` page
  in the same change and leave nothing behind in the source.
- Machine-read directives are not comments and stay: build tags, `//go:embed`,
  `//go:generate`, linter pragmas (`//nolint`, `// eslint-disable-next-line`,
  `// ignore:`), license headers, codegen "do not edit" banners, and generated
  files as a whole.
- Commit messages, PR descriptions, and test names carry the narration that
  used to go in comments.

This rule is repeated verbatim in every mwanachama repo's `CLAUDE.md` so that
it reaches sessions that do not load this machine's user-level config —
scheduled cloud routines, other machines, and other agent harnesses.
