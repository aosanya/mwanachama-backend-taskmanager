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
| `case.closed` | the last open task settled, or `EndCase` | `case_id`, `status`, `closed_at` |

## Ending a case by hand (W73, decisions 416 and 419)

`POST /cases/{caseID}/end` (`EndCase`, action `taskmanager.case.end`) takes
`status` (`done` or `cancelled`) and an optional `reply`, stored on the case
as what the requester is told. It works on a case with no tasks, which
otherwise stays open until ended.

- `done` is refused with `ErrCaseHasOpenTask` (409) while any live linked
  task is neither completed nor cancelled.
- `cancelled` moves the case first, under `status = open`, then cancels every
  open linked task through the same path the run cascade uses; because the
  case is no longer open, those cancellations cannot turn it into `done`.
- Anything else is `ErrInvalidCase` (400); a case already ended is
  `ErrCaseNotOpen` (409). Both publish `case.closed` with the status given.
- A rollback never reopens a done case and a failed task does not settle one;
  both were already so and are unchanged.

`GET /cases/mine` (`ListMyCases`, action `taskmanager.case.list_mine`) lists
the cases the caller opened, the requester bound `from: caller`, so a
`requester_id` in the query is overwritten. `GET /cases` stays the staff list.

`DeleteTask` re-checks the task's case after the soft delete, and the check
closes a case only when at least one live linked task remains and every one
is settled. Deleting a case's only task leaves it open; deleting its last
open task beside a settled one closes it `done`.

Freeing an appointment when a case is cancelled is the host's (wakala-api
WK198), not this module's.

## Votes (HS52, hub decision 17)

Members rank open cases by voting on them. A third role, `case_vote`, holds one
row per member per case: `vote_key` (a digest of the case and the member, the
primary key, so the database itself allows a member one vote per case),
`case_id`, `member_id`, `cast_by` and `voted_at`.

| Rule | How it holds |
| --- | --- |
| One vote per member per open case | the key; voting again changes nothing and the first `cast_by` stands |
| A member can take the vote back | `TakeBackVote` deletes the row; a second take back is `ErrVoteNotFound` (404), and a later vote is a fresh one |
| The requester counts as the first vote | not stored: every case reads as stored votes plus one. Voting as the requester adds nothing; taking it back is refused with `ErrRequesterVoteStays` (409), a reading of the rule the owner has not confirmed |
| Owners and agents are refused | `member_kind` is `member` or left out; anything else is `ErrVoterNotMember` (403). This module has no auth model and cannot tell what a member is, so the host fills `member_kind` from the member's role in the agency and must not take it from the caller's body |
| A vote records the member and who cast it | `member_id` is the body's member, or the caller when none is given; `cast_by` is always the caller. Hub Steward casting for the member who said "me too" shows `member_id` = the member and `cast_by` = Hub Steward |
| Only open cases take or lose votes | `ErrCaseNotOpen` (409) on a case that is done or cancelled |

Addresses, each answering with the case and its `votes` (a ranked case) except the list of votes:

| Address | Call | Action |
| --- | --- | --- |
| `POST /cases/{caseID}/votes` | `VoteCase` | `taskmanager.case.vote` |
| `DELETE /cases/{caseID}/votes/{memberID}` | `TakeBackVote` | `taskmanager.case.take_back_vote` |
| `GET /cases/{caseID}/votes` | `ListCaseVotes` | `taskmanager.case.list_votes` |
| `GET /cases/ranked` | `ListCasesByVotes` | `taskmanager.case.list_by_votes` |

`ListCasesByVotes` takes the same narrowing as `GET /cases`, defaults `status`
to `open`, and orders by votes (most first), then the earlier opened, then id.
It ranks within the 500 most recently opened cases the filter matches, the
page every list in this module is capped at.

The action gate is the host's place to say who may vote at all and who may cast
for someone else: both go through `taskmanager.case.vote`, so a host that wants
the second restricted must check `member_id` against the caller itself.

## Addresses

`POST /cases`, `GET /cases`, `GET /cases/{caseID}`,
`PUT /cases/{caseID}/tasks/{taskID}`, `GET /cases/{caseID}/tasks`, gated by the
actions `taskmanager.case.open`, `.list`, `.read`, `.link_task` and
`.list_tasks`; `POST /cases/{caseID}/end` and `GET /cases/mine` by `.end`
and `.list_mine`.

## Not built

