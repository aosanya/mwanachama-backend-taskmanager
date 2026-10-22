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

## Not built yet

- A declared `repeating_task` object in `taskmanager.blueprint.json`, its
  `models/` type, and a record of which period keys were made (the store-side
  idempotency), plus the declared operations to create, list and stop one.
  Storing a `recur.Schedule` needs a serialised form (rule string, start,
  end, zone name, dates).
- Assigning the created task: `AssignTaskToHolder` exists and takes the
  template's kind and id.

## Open questions

- A schedule's last period when the schedule has no end after it: no due time
  can be derived. This slice makes no task; the owner may want one due at the
  schedule's end or with no due time.
- Whether a stopped template may be resumed, and if so whether periods missed
  while stopped are made.
- When a role is the assignee, whether the task stays on the role or goes to
  one holder of it.
