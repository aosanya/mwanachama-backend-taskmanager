# Declared objects

The tables come from a JSON spec, not from Go structs. This repo adopted
`mwanachama-backend-catalog`'s shape on 2026-09-28; the engine it runs on is
`mwanachama-backend-shared/spec`, `specstore` and `dispatch`, and the
org-wide rationale is in
`developer/documentation/2. design/architecture-spec-driven-modules.md`.

## The three files

| File | What it is |
| --- | --- |
| `taskmanager.blueprint.json` | The module's fourteen objects, declared once. Reached through `Blueprint()`, `LoadSpec(path)` and `ParseSpec(raw)`. |
| `taskmanager.operations.json` | The route table. Reached through `Operations()`, built by `shared/dispatch`. See [routes.md](routes.md). |
| `spec/examples/*.json` | The same module under two domains, each naming the objects in its own words. |

Load a domain spec through `LoadSpec`/`ParseSpec`, never through
`spec.Load` — a roled object loaded directly arrives with no fields,
because the fields are the blueprint's.

## The fourteen roles

A rule reaches its table through a role, never through a domain's own noun.

| Role | What it holds |
| --- | --- |
| `task` | One unit of work: what is to be done, who holds it, what it waits on. |
| `task_todo` | One ordered step a task was decomposed into. |
| `agent` | A worker that tasks are assigned to. |
| `project` | A container grouping related tasks. |
| `tag` | A free-form label. |
| `deliverable` | Something a task or step must produce. |
| `acceptance_criteria` | A condition checked before the work is accepted. |
| `workflow_run` | The anchor of one orchestrated execution. |
| `import_job` | One asynchronous import of a project and its tasks. |
| `blocker` | A hard gate between two tasks. |
| `dependency` | A soft ordering between two tasks. |
| `membership` | One task's membership of one project. |
| `tagging` | One label attached to one task. |
| `code_sequence` | The next number to mint for one kind of thing's stable code. |

`store.go` holds the role constants and `carriers()`, the map from each role
to the Go type that carries its columns. `NewTaskManager` checks the spec and
the types against each other and refuses to build rather than dropping a
value on every write.

## A domain names objects; it does not re-declare them

A domain spec supplies `instance`, the name and table each role lands in, its
own indexes, and a default on a declared field — nothing else. Setting a
type, a description or a value set on a field the module declares is refused
by name at load. A domain that needs a field of its own declares an object of
its own, with no role.

Two are shipped:

| Role | `work.taskmanager.json` (instance `kazi`) | `agency.taskmanager.json` (instance `wakala`) |
| --- | --- | --- |
| `task` | `task` | `work_item` |
| `task_todo` | `task_todo` | `step` |
| `agent` | `agent` | `operator` |
| `project` | `project` | `objective` |
| `tag` | `tag` | `label` |
| `deliverable` | `deliverable` | `output` |
| `acceptance_criteria` | `acceptance_criteria` | `check` |
| `blocker` | `task_block` | `gate` |
| `dependency` | `task_dependency` | `precedence` |
| `membership` | `task_project_membership` | `objective_membership` |
| `tagging` | `task_tag` | `labelling` |

`TestShippedExamplesCoexist` migrates both into one database.

`agency.taskmanager.json` is also the shipped domain: `SpecFor(instance)`
embeds it and swaps the instance, which is how `mwanachama-wakala-api` gives
each registered agency its own board.

## A table is `<instance>_<module>_<object>`

`wakala_taskmanager_work_items`. The module segment is not decoration: the
agency module also declares a `work_item`, and without the segment a
taskmanager instance named `wakala` and an agency instance of the same name
both want `wakala_work_items`. Neither module would notice, because
`AutoMigrate` adopts a table that already exists and
`create table if not exists` is a no-op against one — a collision here is
silent data mixing, not an error. `TestTheModuleSegmentKeepsTwoModulesApart`
holds that name in place.

Every emitted name is measured against Postgres's 63-byte limit when the spec
loads, because Postgres truncates past it without complaining. The longest
name this module emits is an index on `objective_memberships`, which fits
with room to spare under wakala-api's 11-character instance slugs.

## A column is found by field name, never by json tag

`specstore.ColumnName` turns `SubmittedBy` into `submitted_by` and `RunID`
into `run_id`. The tag is a presentation choice and would get this wrong
where it hurts.

Every declared column is written on every write. A map missing a key means
"leave it alone" to an update, so omitting empty values would make clearing a
field impossible.

## What changed in the Go types

The conversion needed the types and the declaration to agree exactly, which
surfaced four disagreements the row structs had been hiding:

- `Task.Deleted`, `TaskTodo.Deleted` and `Project.Deleted` are now fields.
  They were columns on the row structs and invisible to the domain types.
- `WorkflowRun.FailureReason` is now a field. It was a write-only column with
  no Go field at all — written on a failed transition for whoever read the
  row outside this module, and never read back.
- `Task.AssignedTo` lands in `assigned_to`. The row struct called the column
  `assigned_agent_id`.
- `Task.Tags` and `ImportProjectJob.ProgressSteps` are declared `json`
  columns. Both were previously derived and not stored — tags resolved from
  the tagging rows on every read, progress kept in memory. The tagging rows
  remain the authority for tags; the column is the denormalised copy, the
  same treatment `Task.AssignedTo` and `Task.WorkflowRunID` already had.
  Progress now survives a restart, which it did not before.

`models.Direction` changed from an `int` to a `string` (`"inbound"` /
`"outbound"`), so the dispatcher binds `?direction=inbound` straight into it
and the address keeps the shape its hand-written handler had. Anything other
than `inbound` reads as outbound, which is the default the old handler
applied.

## What is superseded

- **`gormstore/`** — deleted 2026-09-28. Its fourteen row structs, their
  to/from converters and `Migrate` are what `spec.Migrate` and `specstore`'s
  codec replace. Root `tables.go` and its `TableNames`/`DefaultTableNames`
  went with it; `NewTaskManager(db, *spec.Spec, pub)` is the constructor now.
  No other repo imported this one, so nothing outside needed a shim.
- **`gormstore.Migrate`'s code backfill** — gone with it. It assigned a
  `Code` to rows predating the column; the spec-driven table set is new, so
  there are no such rows. `nextCode` itself moved to root `codesequence.go`
  unchanged, W22's missing CAS guard included — that is a filed bug, not
  something this conversion quietly fixed.
- **`routes/`'s thirteen hand-written route builders** — deleted the same
  day. See [routes.md](routes.md).
