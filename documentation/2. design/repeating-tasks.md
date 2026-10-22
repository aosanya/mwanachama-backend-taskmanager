# Repeating tasks (W72, first slice)

Member app decision 338: a repeating task is a template with a schedule and an
assignee (a person or a role, decision 272). At the start of each period a new
task is made, due by the period's end, whether or not the last one was done.
Stopping it keeps every task it already made. W72 takes up W45 and W63.

## What this slice is

`repeating.go` holds the rule, in Go, with no storage:

- `RepeatingTask` — id, title, description, a `recur.Schedule` from
  `mwanachama-backend-shared/recur` (S66), an assignee kind and id, and
  `Stopped`. `Validate` accepts only an actor or a role as assignee; an agent
  is refused, because decision 272 names a person or a role.
- `OccurrenceAt(at)` — the period `at` falls in, from `Schedule.Period`. The
  task's due time is the period's end, which is the next occurrence's start.
  A stopped template, a moment before the schedule starts, or a last period
  with no end answers "nothing due".
- `Occurrence.PeriodKey` — `<template id>@<period start, UTC RFC 3339>`. Any
  moment inside one period yields the same key, so a host that creates a task
  only for a key it has not seen makes exactly one task per (template, period)
  however often it ticks.
- `Occurrence.Task(r)` — the task to create: the template's title and
  description, `due_at` at the period's end.
- `Stop()` — sets `Stopped`. It touches nothing already created, which is
  what keeps past tasks.

Whether the previous period's task was done is never read, so a missed period
does not hold up the next one.

## Who ticks

The package has no scheduler (`workflow_run_watchdog.go` already says so).
Calling `OccurrenceAt(now)` on a timer, and creating the task for a key not yet
seen, is the host's.

## Stored (second slice)

Two declared objects in `taskmanager.blueprint.json`, named in both shipped
domain specs:

- `repeating_task` (`models.RepeatingTaskRecord`) — the template. The
  `recur.Schedule` is stored in a serialised form: `schedule_rule` (the RFC 5545
  rule, `Rule.String()`), `schedule_dates` (YYYY-MM-DD), `schedule_start` and
  `schedule_end` as UTC instants, and `schedule_zone` as the IANA zone name.
  `RecordOf` and `RepeatingTaskFromRecord` convert both ways; a zone the host
  cannot load is refused. Stopping writes `stopped_at`; the row is never
  deleted. `assignee_kind` is a plain string rather than an enum, because only
  `actor` and `role` are allowed here and `Validate` already holds that.
- `repeating_task_period` (`models.RepeatingTaskPeriod`) — one row per period
  made, keyed by the period key, with the task it made and its due time.

The manager methods, on `TaskManager`:

- `CreateRepeatingTask` mints the id, validates, stores.
- `GetRepeatingTask`, `ListRepeatingTasks` (by assignee kind and id).
- `StopRepeatingTask` sets `stopped_at` once; stopping again is a no-op.
- `MakeRepeatingTaskDue(id, at)` — the host's tick. It finds the period `at`
  falls in; if that key has a row it answers the row and `made = false`;
  otherwise it creates the task, assigns it to the template's assignee through
  `AssignTaskToHolder` (the role stays the assignee, which is the provisional
  answer to the open question below), and records the period. A stopped
  template or a period with no end makes nothing.

The task creation, the assignment and the period row are one transaction.
The tick builds a store over the transaction from the manager's own spec and
runs `CreateTask` and `AssignTaskToHolder` through it, so a failure at any of
the three leaves no task and no period row, and the next tick makes the
period's task cleanly. The events those two calls raise (task created, task
assigned) are held during the transaction and published only after it
commits, so a rolled-back tick announces nothing.
`TestATickWhosePeriodRowFailsLeavesNoTaskBehind` refuses the period row's
insert and checks no task is left; it fails against the three-write version.

## Declared operations (third slice)

Three addresses in `taskmanager.operations.json`, each gated on its own
action:

| Address | Call | Action |
| --- | --- | --- |
| `GET /repeating-tasks/{repeatingTaskID}` | `GetRepeatingTask` | `taskmanager.repeating_task.read` |
| `GET /repeating-tasks` | `ListRepeatingTasks` (query `assignee_kind`, `assignee_id`) | `taskmanager.repeating_task.list` |
| `POST /repeating-tasks/{repeatingTaskID}/stop` | `StopRepeatingTask` | `taskmanager.repeating_task.stop` |

`ErrRepeatingTaskNotFound` answers 404 and `ErrInvalidRepeatingTask` 400.
They are pinned over a real mux by
`routes/repeating_task_test.go`.

## Not built yet

- **Create over the wire.** `CreateRepeatingTask` takes a `RepeatingTask`,
  whose `recur.Schedule` has no JSON form (a `*recur.Rule`, `recur.Date`s and
  a `*time.Location`). Declaring it needs a body shape — most likely the
  stored record's own `schedule_rule`/`schedule_dates`/`schedule_start`/
  `schedule_end`/`schedule_zone` strings, converted through
  `RepeatingTaskFromRecord` — and a manager method taking that shape. Not
  declared in this slice.
- **The tick over the wire.** `MakeRepeatingTaskDue` takes a `time.Time` and
  answers a period and a `made` flag. It is the host's timer, not a caller's
  act, so whether it should be an address at all, and with which action, is
  left to the host that runs the timer; nothing declares it yet.

## Open questions

- A schedule's last period when the schedule has no end after it: no due time
  can be derived. This slice makes no task; the owner may want one due at the
  schedule's end or with no due time.
- Whether a stopped template may be resumed, and if so whether periods missed
  while stopped are made.
- When a role is the assignee, whether the task stays on the role or goes to
  one holder of it.
