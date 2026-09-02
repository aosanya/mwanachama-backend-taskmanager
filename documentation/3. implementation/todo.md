# mwanachama-taskmanager (Go)

Open tasks only — 🚀 In Progress · 📋 Not Started · ⏸️ Blocked.
Everything else (completed rows, board context) is in [todo_done.md](todo_done.md).

| Task | Title | Status | Depends on |
|------|-------|--------|------------|
| W10 | Wire into `mwanachama-api-gateway`: new `internal/domain/…` consuming `TaskManager`, constructed via `postgres.NewBackend(db, postgres.DefaultTableNames("work_"))` + `NewTaskManager`; owns the versioned `work_*` migration (`postgres.DDL(postgres.DefaultTableNames("work_"))` embedded in its `cmd/migrate`); check `internal/domain/agentic` for naming/overlap first | 📋 | W9 |
