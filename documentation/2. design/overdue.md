# The overdue list

W33. A board holds the clock and every task holds its own `due_at`, but
nothing read the two together, so the list of late work was assembled by
whoever remembered. `ListOverdueTasks` is that read, declared as
`list_overdue_tasks` (`GET /overdue-tasks`, action
`taskmanager.task.list_overdue`).

## What it carries

- **Open work only.** A task in `completed`, `failed` or `cancelled` is never
  overdue, whatever its date. Every other status, `blocked` and
  `awaiting-direction` included, is still somebody's to finish.
- **Only a task with a due date.** An empty `due_at`, or one that does not
  parse as RFC 3339, is skipped rather than refused, so one bad row cannot
  empty the list.
- **Past the grace.** A task is carried once `due_at` is more than the
  agency's `overdue_grace_hours` behind the clock.
- **Longest overdue first**, each row with `overdue_hours`, the whole hours
  since `due_at` (not since the grace ran out), capped by `limit` like every
  other listing.

## The grace is a declared setting

`overdue_grace_hours` is declared on the blueprint beside `task_states`
(shared's S64): an `int`, default `0`. `NewTaskManager` resolves it once and
refuses a negative value, the same way it resolves `task_states`. The
definition and override layers wait on S64's storage, as they do for
`task_states`.

## The clock is the caller's

`as_of` (RFC 3339) reads the board at another instant; omitted, it is now.
That is what the tests set, and it is also what lets a scheduled routine
re-run a missed sweep for the instant it should have run. An unreadable
`as_of` is `ErrInvalidOverdueFilter`, a 400.

## What is the host's

The read writes nothing. Running it on a schedule and opening a follow-up
task per overdue row (the municipality template's wish, AGX14) is the
host's: a cron or routine that calls this operation and then `create_task`.
No scheduler lives in this module.
