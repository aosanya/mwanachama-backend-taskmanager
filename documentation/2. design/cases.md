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
- Only `UpdateTask` closes a case. A task settled by the run-cancel cascade or
  a rollback does not yet.
- `GET /cases` is not narrowed to the caller; a host wanting a requester to see
  only their own cases has to bind the filter.
