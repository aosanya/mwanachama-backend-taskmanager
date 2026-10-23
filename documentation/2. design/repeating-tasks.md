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
| `POST /repeating-tasks` | `StartRepeatingTask` (body `NewRepeatingTask`, 201) | `taskmanager.repeating_task.create` |
| `GET /repeating-tasks/{repeatingTaskID}` | `GetRepeatingTask` | `taskmanager.repeating_task.read` |
| `GET /repeating-tasks` | `ListRepeatingTasks` (query `assignee_kind`, `assignee_id`) | `taskmanager.repeating_task.list` |
| `POST /repeating-tasks/{repeatingTaskID}/stop` | `StopRepeatingTask` | `taskmanager.repeating_task.stop` |

`ErrRepeatingTaskNotFound` answers 404 and `ErrInvalidRepeatingTask` 400.
They are pinned over a real mux by
`routes/repeating_task_test.go`.

## Create over the wire (fourth slice)

`POST /repeating-tasks` takes `models.NewRepeatingTask`: `title`, optional
`description`, `assignee_kind` (`actor` or `role`), `assignee_id` and a
`schedule` object. The schedule is the same body shape as
`mwanachama-backend-forms`' `form.schedule` (its `schedule.md`), so a staff
screen that already composes one composes the other:

| Field | Shape |
| --- | --- |
| `start` | RFC 3339 instant |
| `end` | optional RFC 3339 instant |
| `zone` | IANA time zone name |
| `rule` | an iCalendar RRULE, or |
| `dates` | a list of `YYYY-MM-DD` |

`models.Schedule` carries it. `RepeatingTaskFromNew` maps it onto the stored
record's `schedule_*` strings and converts through `RepeatingTaskFromRecord`,
so the wire and the store parse one way; `StartRepeatingTask` then calls
`CreateRepeatingTask`, which mints the id and validates. An unknown zone, an
instant that is not RFC 3339, an unparseable rule or date, an agent assignee
or a schedule `recur` refuses all answer 400 (`ErrInvalidRepeatingTask`). The
stored record keeps its flat `schedule_*` fields on read; only the create
body nests them. Pinned over a real mux by
`TestARepeatingTaskIsCreatedOverTheMuxWithTheFormsScheduleShape`. A host must
grant `taskmanager.repeating_task.create` before anyone can reach it.

Unlike forms, an empty `zone` is not refused here: `time.LoadLocation("")`
is UTC, which is what the stored form already did.

## Settled by the owner (fifth slice, decisions 362, 363, 366, 367)

- **A list of dates needs an end.** `Validate` refuses a schedule of dates
  with no `end` (`ErrInvalidRepeatingTask`, 400 over the wire). The last
  date's period runs to the end, so its task is made and is due on the end;
  before this the last date's period had no due time and its task was never
  made. A rule with no end is still accepted: every period of it has a next
  occurrence to end on. A template stored before this change, with dates and
  no end, now answers the tick with `ErrInvalidRepeatingTask`.
- **Stop is final.** There is no resume (as built).
- **No back-fill.** Only the period the tick's moment falls in is made
  (as built).
- **The tick stays the host's Go call**, with no address (as built).
- **A role assignee stays on the role** (as built).

### For the host's sweep (wakala-api WK203)

- `ListRunningRepeatingTasks(ctx) ([]RepeatingTaskRecord, error)` on
  `TaskManager` reads every template whose `stopped_at` is unset, oldest
  first, with no page cap; `ListRepeatingTasks` keeps its cap of 500. It is
  not declared as an operation: it is for the host's timer, not a caller.
- **A lost race answers "already made".** Two ticks for the same period both
  pass the first read; the loser's period row collides with the winner's on
  the primary key and its transaction rolls back. `MakeRepeatingTaskDue` then
  reads the period again and, finding the winner's row, answers it with
  `made = false` and no error, exactly as a tick that came second would. Any
  other failure is still returned.

Pinned by `repeating_dates_test.go`: the refusal, the last date's task due on
the end, a sweep over 503 templates that skips the stopped one, and a rival
tick run between the first read and the transaction.

## The schedule body is shared's `recur.Wire`

Since shared's S67, a create body's `schedule` is `recur.Wire` — `start`,
`end`, `zone`, `rule` or `dates`, and `open_days` — the same body forms and
comm read, so the studio's one Repeat picker serves all three. The field
names did not change and no address moved; taskmanager's own copy of the
shape, `models.Schedule`, is gone.

`RepeatingTaskFromNew` runs `Wire.Schedule()` first, so a list of dates with
no end and every other malformed schedule is refused by the shared reader
and answered as `ErrInvalidRepeatingTask`. `open_days` is refused when set:
a repeating task's made task is due at its period's end, the record has no
column for it, and storing a schedule that silently drops a field would lie.
The stored record keeps its five `schedule_*` columns.

Pinned by `repeating_wire_test.go`.

## Questions settled

The three questions this page used to carry (the last period with no end,
resuming a stopped template, a role assignee) are answered above.
