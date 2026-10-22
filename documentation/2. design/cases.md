# Cases

A **case** is one request somebody brought, worked through the tasks linked
to it (W29, member app decision 276). It is declared in
`taskmanager.blueprint.json` as two roles, appended together at the end of the
object list:

- `case` — the requester, the subject the case is about as two opaque strings
  (`subject_kind`, `subject_id`, never interpreted here), a title, a `status`
  of `open`, `done` or `cancelled`, and `opened_at`/`closed_at`.
- `case_task` — one row per task worked for a case, keyed by `task_id`, so a
  task serves at most one case.

A link table rather than a `case_id` column on `task` keeps the `task` object
untouched, and makes "this task serves one case" the primary key rather than a
rule somebody has to remember.

## Who owns what

`OpenCase(ctx, requesterID, c)` takes the requester from the caller
(`{"from": "caller"}` in the operations file) and overwrites anything the body
claims. The id, the status (`open`) and the timestamps are the store's.

## When a case closes

`UpdateTask` calls `closeCaseIfSettled` after any transition into `completed`
or `cancelled`. When every task linked to that task's case is `completed` or
`cancelled`, the case moves to `done`, `closed_at` is stamped and
`case.closed` is published. The write is guarded on `status = 'open'`, so two
tasks settling together publish once. A `failed` task does not settle a case:
the row names done and cancelled only. A case with no tasks stays open.

### Every path that writes a task's status

A task's status is written in five places, and each one either settles its
case or provably cannot:

| Path | Writes | Settles the case |
| --- | --- | --- |
| `UpdateTask` | any legal transition | yes, after the write |
| `cancelTask`, the `CancelWorkflowRun` cascade | `cancelled` | yes, after each task's write |
| `HandleTaskTimeout`, `HandleRunTimeout` (watchdog) | `failed`, through `UpdateTask` | goes through `closeCaseIfSettled`, which does nothing for `failed` |
| `UnblockTask` | `awaiting-direction`, through `UpdateTask` | cannot: not a settling status |
| `setTaskStatus` (`AssignTask`'s block, the unblock cascade) | `blocked`, `pending` | cannot: not a settling status |
| `DeleteWorkflowRunArtifacts` (rollback) | `pending` | cannot: not a settling status |

None of the settling writes runs inside a transaction today: `UpdateTask` and
`cancelTask` each write one row and then settle, the same way. The only
transactional status write is the rollback's reset, and a reset to `pending`
never closes a case. Nor does it reopen one: a case already `done` stays
`done` when a rollback puts one of its tasks back to `pending`, and an open
case stays open. Whether a rollback should reopen a closed case is an open
question for the owner, not settled here.

Each row is pinned in `case_status_paths_test.go`.

Linking a task to a case that is not open is refused (`ErrCaseNotOpen`, 409);
linking a task already serving another case is refused
(`ErrTaskAlreadyInCase`, 409); relinking the same pair is a no-op.

## Events

| Topic | When | Payload |
| --- | --- | --- |
| `case.opened` | `OpenCase` stored the row | `case_id`, `requester_id`, `subject_kind`, `subject_id` |
| `case.closed` | the last open task settled | `case_id`, `status`, `closed_at` |

## Addresses

`POST /cases`, `GET /cases`, `GET /cases/{caseID}`,
`PUT /cases/{caseID}/tasks/{taskID}`, `GET /cases/{caseID}/tasks`, gated by the
actions `taskmanager.case.open`, `.list`, `.read`, `.link_task` and
`.list_tasks`.

## Not built

- Nothing moves a case to `cancelled`; the value is declared, no operation
  sets it.
- `GET /cases` is not narrowed to the caller; a host wanting a requester to see
  only their own cases has to bind the filter.
