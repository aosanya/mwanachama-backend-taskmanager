# CLAUDE.md

Guidance for Claude Code working in this repository.

## Project: mwanachama-backend-taskmanager

Postgres port of `CodeValdWork` for [mwanachama-frontend-kazi](../mwanachama-frontend-kazi).
Module path `github.com/aosanya/mwanachama-backend-taskmanager`.

Dropped from the original: `proto/`, `cmd/server`, `internal/server` (gRPC
`TaskServiceServer` + the ~1150-line `TaskEventDispatcher` that drives
AI-failure-recovery orchestration off inbound events), `internal/registrar`
(CodeValdCortex Cross heartbeat). `mwanachama-backend-api-gateway` runs as one
service and imports this package directly — there is no separate process to
dispatch events to. If the escalation/retry orchestration the dispatcher did
is still needed, it has to be re-homed as in-process logic here or in the
gateway, not assumed to exist.

## Porting notes

- `task.go`'s `TaskManager` interface and `models.go`'s domain types (Task,
  Agent, Project, WorkflowRun, Deliverable, AcceptanceCriteria, Tag,
  TaskTodo) port unchanged, **including** the hand-rolled `CanTransitionTo`
  state machines (Task: 7 states incl. `blocked`/`awaiting-direction`/
  `split`; WorkflowRun: 9 states incl. `paused`/`cancelling`/`rolling_back`/
  `rollback_failed`) — these are pure Go, no storage dependency.
- `schema.go`'s `DefaultWorkSchema()` ports onto `mwanachama-backend-shared`'s
  type-definition shape; vertex uniqueness (e.g. Agent by `agent_id`, Tag by
  `name`) must become real Postgres unique indexes.
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

- Task status lives on
  [documentation/3. implementation/todo.md](documentation/3.%20implementation/todo.md).
- Four-phase `documentation/` layout — see
  [documentation/README.md](documentation/README.md).
- Before wiring into `mwanachama-backend-api-gateway`, check
  `internal/domain/agentic` there for naming/scope overlap.
