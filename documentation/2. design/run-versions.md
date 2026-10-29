# A run keeps its version

HS300, decisions 91 and 95. An agency publishes its workflows as numbered
versions. A run that began on version 1 finishes on version 1, however many
versions are published after it, and going back to an earlier version stops
nothing.

## What a run carries

- `workflow_code`: the workflow it carries out, by the code the agency gave it.
- `workflow_version`: the published version it began on. Written once by
  `StartWorkflowRun` (`POST /workflow-runs/start`, action
  `taskmanager.workflow_run.start`) and never rewritten: every later write goes
  through `saveRun`, which sends the whole row back unchanged in these two
  fields. A run opened by `CreateWorkflowRun` names no workflow and has no
  version.
- `under_way`: true from the moment the run is opened until it reaches an end
  status (`completed`, `failed`, `rolled_back`, `rollback_failed`,
  `cancelled`). `saveRun` derives it from the status on every write, so no
  status change can leave it behind.
- `restarted_from_run_id`: set on a run opened by Stop with a restart.

## The one index on runs still going

`under_way_workflow` is a partial index on `(workflow_code,
workflow_version)` where `under_way` is true. It holds only the runs still
going, so it stays small however many runs have ended, and answers both reads
the Workflows page needs: the orders under way for one workflow with the
version each is on, and which versions are still in use. The blueprint's
predicates cannot say "status is not an end status", which is why `under_way`
is a stored flag and not a condition on `status`.

## The list

`ListWorkflowRunsUnderWay` (`GET /workflow-runs/under-way`, action
`taskmanager.workflow_run.list_under_way`) returns the runs still going, newest
first, narrowed to one workflow by `workflow_code`. A run is going while it is
pending, in progress, paused, cancelling or rolling back.

## Stop, and start again on the good version

`StopWorkflowRun` (`POST /workflow-runs/{runID}/stop`, action
`taskmanager.workflow_run.stop`) ends a run at once as `cancelled`, with the
caller as `cancelled_by` and the reason in `cancel_reason`. There is no
quiesce window and no `cancelling` stop on the way, because the hub manager's
Stop is immediate. Every task the run holds that has not finished is cancelled,
and `work.run.cancelled` is published. A run that has ended, or is rolling
back, is refused with `ErrCannotCancelTerminalRun` (409).

`restart_on_version`, when above zero, then opens a new run of the same
workflow on that version, with the stopped run's trigger event and initiator
and `restarted_from_run_id` pointing back. The request is checked first: a
restart on a run that names no workflow is refused with `ErrInvalidTask` and
stops nothing. If the new run cannot be opened after the stop, the stop stands
and the error comes back with it; stopping again is refused, so the caller
opens the run itself.

## What the publication side owes

A publication is never deleted: archiving moves it off its channel and nothing
else, so a run's version stays readable for as long as the run is going and
after. The agency repo holds a test for that.
