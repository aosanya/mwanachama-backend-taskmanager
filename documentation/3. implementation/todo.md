# mwanachama-taskmanager (Go)

Open tasks only — 🚀 In Progress · 📋 Not Started · ⏸️ Blocked.
Everything else (completed rows, board context) is in [todo_done.md](todo_done.md).

| Task | Title | Status | Depends on |
|------|-------|--------|------------|
| W2 | Port `task.go`'s `TaskManager` interface + `models.go` domain types (Task/Agent/Project/WorkflowRun/Deliverable/AcceptanceCriteria/Tag/TaskTodo), incl. `CanTransitionTo` state machines, unchanged | 📋 | W1 |
| W3 | Port `schema.go`'s `DefaultWorkSchema()` onto `mwanachama-go-shared`'s type defs | 📋 | W2, mwanachama-go-shared#S2 |
| W4 | Port straightforward impl files as-is: task CRUD, converters, project, assignment(+unblock), deliverable, relationship engine, todo, agent | 📋 | W3, mwanachama-go-shared#S4 |
| W5 | Port `WorkflowRun` subsystem (create/get/list/closure, cancel+quiesce, failure-budget, watchdog) — closure/cascade queries rebuilt as recursive CTEs | 📋 | W4 |
| W6 | Port rollback (`RollbackWorkflowRun`'s 4-step compensation) with real Postgres transactions (`pgx.Tx`) — new transactional design, not a mechanical port | 📋 | W5 |
| W7 | Wire `events.go`'s ~25 topics to `mwanachama-go-shared`'s `Publisher` interface | 📋 | W2, mwanachama-go-shared#S7 |
| W8 | Port `import.go` (project-bootstrap-from-JSON) | 📋 | W4 |
| W9 | Unit tests ported/adapted; integration tests against real Postgres | 📋 | W5, W6, W8 |
| W10 | Wire into `mwanachama-api-gateway`: new `internal/domain/…` consuming `TaskManager`; check `internal/domain/agentic` for naming/overlap first | 📋 | W9 |
