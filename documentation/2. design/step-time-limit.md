# A waiting step has a time limit

HS304, decisions 99 and 87. An agency can give a step that waits on a person a
time limit in the workflow editor (5 minutes, 15 minutes, as long as it
wants). When the limit passes with no answer the run closes, the workflow's
cancel workflow opens, and the cancel workflow's notify step tells the person
who was waiting. A step with no limit waits for ever. The limit is the
agency's: this module holds no default.

The limit and the cancel workflow are set on the step in the agency repo
(`time_limit_minutes` and `cancel_workflow_id` on a work item, see its
`step-time-limit.md`). This module holds the clock.

## What a run carries

- `current_step_id`, `current_step_started_at`: the step the run waits at and
  when it began, written by `BeginWorkflowRunStep`.
- `step_due_at`: when the limit passes, an RFC 3339 instant in UTC. Empty when
  the step has no limit. It stays on a closed run so a screen can say when it
  closed.
- `step_cancel_workflow_code`: the cancel workflow to open when the limit
  passes.
- `cancel_run_id`: the run of the cancel workflow, written when the run
  closes.

`closed` is a run status and an end status. It is reached from pending,
in progress and paused, and only by the sweep: `UpdateWorkflowRunStatus`
refuses to set it, the same way it refuses the rollback states.

## The three calls

`BeginWorkflowRunStep` (`POST /workflow-runs/{runID}/step`, action
`taskmanager.workflow_run.begin_step`) records the step the run now waits at.
`limit_minutes` and `cancel_workflow_code` come together or not at all: a
limit with no cancel workflow, a cancel workflow with no limit and a negative
limit are `ErrInvalidTask` (400). Beginning another step replaces the previous
step's limit, so a run moving to a step with no limit stops being watched. A
run that has ended is refused with `ErrCannotCancelTerminalRun` (409).

`AnswerWorkflowRunStep` (`POST /workflow-runs/{runID}/step/answered`, action
`taskmanager.workflow_run.answer_step`) clears the limit once the person
answered. It is idempotent.

`CloseWorkflowRunsPastLimit` (`POST /workflow-runs/close-past-limit`, action
`taskmanager.workflow_run.close_past_limit`) is the sweep. It takes an
optional `as_of` instant, which a test uses as its clock and the caller leaves
out. It returns the runs it closed.

## What the sweep does

For each run under way whose `step_due_at` is at or before the instant,
oldest first:

1. Opens the cancel workflow's run on the version the closing run began on,
   with trigger `step_limit_passed:<run id>` and initiator `watchdog`. Its name
   is the closing run's name plus `-closed`; a name already taken means a
   previous sweep opened it and failed before closing, so that run is reused
   and no second cancel opens.
2. Closes the run: status `closed`, `cancelled_by` `watchdog`, `cancel_reason`
   saying which step and which instant, `cancel_run_id` set, `under_way` false.
3. Cancels the tasks the run holds that have not finished.
4. Publishes `run.closed` with the run, the step, the due instant and the
   cancel run.

A failure on one run is reported with its id and does not stop the others;
the run is picked up again by the next sweep. Running the sweep twice closes
a run once.

## The closed list

HS426. `ListWorkflowRunsUnderWay` never returns a closed run, because a
closed run has `under_way` false. `ListWorkflowRunsClosed`
(`GET /workflow-runs/closed`, action `taskmanager.workflow_run.list_closed`,
tool `taskmanager_workflow_run_list_closed`) returns the runs whose status is
`closed`, the latest closing first, narrowed to one workflow by the optional
`workflow_code`. Each row is the whole run, so a screen reads `status`,
`completed_at` (the instant the sweep closed it), `step_due_at`,
`cancel_run_id`, `cancel_reason`, `workflow_code` and `workflow_version` from
it. Only the sweep makes a run `closed`, so a run stopped by a person
(`cancelled`), completed or failed never appears here. The cancel run itself is
an ordinary run under way and is listed by `ListWorkflowRunsUnderWay`. The list
holds at most one page (`maxListPage`) and reads the `status` index. The
mounting service needs no change: it builds its tools from the operations file.

## The index, and why not the old watchdog

`step_due` is a partial index on `step_due_at` where `under_way` is true and
`step_due_at` is not empty. It holds only the runs still waiting under a
limit, so a sweep reads those and nothing else however many finished runs the
table holds. The older `ListWorkflowRunsStaleSince` cannot serve this: it
drops finished runs in Go after taking 500 rows, so after 500 finished orders
it can see none of the waiting ones (hub `review/engine.md`). A test pins the
new sweep with 600 finished runs ahead of the waiting one.

## What the notify step needs from here

The Customer's Inbox row ("Nobody answered") is the cancel workflow's notify
step, which runs as the run `cancel_run_id` names. The screens read the limit
from `step_due_at`, the Closed state from `status`, and when it closed from
`completed_at`.

## Not here

- Nothing in this repo calls the sweep: no scheduler lives here. The
  mounting service calls it once a minute for each agency that has a step with
  a limit (hub question Q90, option a).
- Nothing in this repo calls `BeginWorkflowRunStep`: whatever moves a run from
  step to step does, with the limit the published step carries.
