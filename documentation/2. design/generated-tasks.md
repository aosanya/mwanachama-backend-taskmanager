# Generated tasks (W34)

Work generated from a list: one task per ward, station, respondent or any
other subject a host can list, made from one template in one call.

## What the module does

`GenerateTasks` (`POST /generated-tasks`, action
`taskmanager.generated_task.create`) takes:

- `run` — an opaque name the host gives this generation, such as the raw key
  of the survey or campaign the list was drawn for;
- `template` — the task fields every generated task shares: `title`
  (required), `description`, `priority`, `due_at` (RFC 3339, normalised to
  UTC) and `estimated_hours`;
- `subjects` — the opaque keys the host listed.

It creates one task per subject in **one transaction**. Each task carries its
subject key as its `linked_ref`, the existing field for "the thing outside
this module this task is about", so no new task column was needed.

## Idempotent per (run, subject)

The `generated_task` role records each `(run, subject)` pair, keyed by a
digest of both, with the task it made. A replay of the same run creates
nothing for a subject already generated and answers it with `created: false`
and the original task id; a new subject on a replay is generated. The same
subject under another run gets its own task. This was chosen over refusing a
replay because a host re-running a group query after a few members joined
should add their tasks and leave the others alone.

## Refusals

The whole call is refused, and nothing is created, when:

- the run or the template title is blank, the due time does not parse, there
  are no subjects, or a subject is blank or repeated in the request —
  `ErrInvalidGeneratedTasks`, 400;
- there are more than `MaxGeneratedSubjects` (5000) subjects —
  `ErrTooManySubjects`, 413. Party-mobilization lists run to thousands, so the
  cap sits above the largest list named so far while keeping one transaction
  bounded. A longer list is split by the host into several runs or several
  calls under one run, which the idempotency makes safe.

## What the module does not do

It never resolves a subject or a run. Which groups or register records a
template fans out over is the host's group or spec query; wakala-api supplies
the list. The template does not name a project and does not assign each task;
both are left for a later row.
