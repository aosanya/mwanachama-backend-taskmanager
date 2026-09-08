// Package mwanachamataskmanager provides task lifecycle management for
// mwanachama-frontend-kazi. It exposes [TaskManager] — the single interface
// for creating, reading, updating, deleting, and listing tasks assigned to
// agents.
//
// Storage is GORM-backed relational tables (moved off
// mwanachama-backend-shared's entitygraph.DataManager, 2026-09-08, following
// mwanachama-backend-actor's earlier migration as the template): domain
// logic and storage both live in this package, imported directly into
// mwanachama-backend-api-gateway — no separate service, no gRPC, no proto.
//
// Layout:
//   - models/    — domain types, one file per entity/value type; re-exported
//     as aliases in aliases.go so callers only need this package's import.
//   - gormstore/ — GORM row structs, row<->domain conversion, table names,
//     migration. Also owns the join tables for the genuinely many-to-many
//     edge labels (blocks, depends_on, member_of, has_tag) — every other
//     edge label denormalises into a plain FK column on one endpoint's row.
//   - routes/    — HTTP handlers over [TaskManager], mounted by the gateway.
//   - doc.go (this file), tables.go — table-name/migrate wrappers.
//   - aliases.go            — models.* type/const aliases.
//   - task.go               — the [TaskManager] interface, the taskManager
//     struct, and [NewTaskManager].
//   - relationship_impl.go  — the generic CreateRelationship/
//     DeleteRelationship/TraverseRelationships dispatcher: each RelLabel*
//     maps to either a denormalised FK write or a join-table row.
//   - task_impl_task.go     — Task CRUD.
//   - project_impl.go       — Project CRUD + membership.
//   - assignment_impl.go    — AssignTask / UnassignTask.
//   - assignment_unblock_impl.go — UnblockDependents / UnblockTask.
//   - agent_impl.go         — UpsertAgent, GetAgent, ListAgents.
//   - todo_impl.go          — TaskTodo CRUD + dispatch.
//   - deliverable_impl.go   — Deliverable / AcceptanceCriteria reads.
//   - workflowrun_impl.go, workflowrun_cancel_impl.go,
//     workflowrun_rollback_impl.go, workflowrun_failure_budget_impl.go,
//     workflowrun_watchdog_impl.go — the WorkflowRun subsystem.
//   - import_impl.go        — ImportProject + async job infrastructure.
//   - errors.go             — sentinel errors.
//   - events.go, failure_recovery.go — event topics/payloads and the
//     direction-form types. Pure Go, no storage dependency — unchanged by
//     the GORM migration.
//
// See this repo's CLAUDE.md for the full migration decision record.
package mwanachamataskmanager
