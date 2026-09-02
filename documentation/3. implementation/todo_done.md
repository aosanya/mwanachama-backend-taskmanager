# mwanachama-taskmanager — completed tasks

| Task | Title | Completed | Notes |
|------|-------|-----------|-------|
| W1 | Bootstrap repo: `git init`, `go.mod`, `.gitignore`, `Makefile`, four-phase `documentation/` skeleton, `todo.md`/`todo_done.md` | 2026-09-02 | New repo (named `mwanachama-taskmanager`, not `mwanachama-tasks`, to avoid clashing with the `mwanachama-kazi` app name). Depends on [mwanachama-go-shared](../../../mwanachama-go-shared) for its entity-graph store (wired via `go.mod` local `replace`). |

## Archived board context

`mwanachama-taskmanager` — Go library, module
`github.com/aosanya/mwanachama-taskmanager`. Consumed by
[mwanachama-api-gateway](../../../mwanachama-api-gateway) for
[mwanachama-kazi](../../../mwanachama-kazi).

### Why this repo exists

`mwanachama-kazi` needs task/workflow management. `CodeValdWork` already
built this — as an ArangoDB-backed library for CodeValdCortex agencies, with
a gRPC service and an event-dispatcher process driving AI-failure-recovery
orchestration. Neither fits `mwanachama-api-gateway`, which is Postgres-only
and runs as one service with no sub-services.

**Decision: full port, same public API surface.** Exploration found
`CodeValdWork`'s business logic (`task*.go`, `workflow_run*.go`) is entirely
storage-agnostic Go calling `entitygraph.DataManager` — the only real new
engineering is the `WorkflowRun` rollback/cascade subsystem, which needs
proper Postgres transactions where the Arango original had none. So the
port is: reuse the `TaskManager` interface/domain types/state machines close
to verbatim, retarget storage onto
[mwanachama-go-shared](../../../mwanachama-go-shared)'s Postgres entity-graph
engine, and drop gRPC/proto/cmd/registrar/the event-dispatcher process.

Full task breakdown and rationale: originally scoped in
`/Users/tony/.claude/plans/kind-snacking-rose.md`.
