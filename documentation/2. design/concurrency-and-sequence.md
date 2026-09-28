# Concurrency and sequence

Six bugs closed on 2026-09-28. Five were the same mistake in five places —
deciding something in Go and then writing as though nothing had moved — and
the sixth was a sequence that existed only as a convention. Each has a test,
and each test was mutation-checked rather than trusted: the guard was removed
and the test was watched to fail.

## A read does not hold a row

`UpdateTask`, `UpdateProject` and `UpdateTaskTodoStatus` each read the row
through a getter that filters `deleted = false`, then wrote through a bare
`WHERE id = ?`. A soft delete landing in the gap was invisible to the write,
which applied the caller's content to a row no read would ever return again
and answered 200.

The write now carries the condition the read relied on:

```
WHERE id = ? AND deleted = false
```

and reports `ErrTaskNotFound` / `ErrProjectNotFound` / `ErrTaskTodoNotFound`
when that matches no row. The condition is the point — not the extra column
in the predicate, but that the write re-checks what the read assumed.

### Why those tests force the interleave

W19's and W20's original pins fired two real HTTP requests at one row, 200
times, and counted how often the persisted row ended up deleted with the
update's content applied. That counts two different things: the defect, and a
perfectly legitimate update-then-delete ordering, which leaves exactly the
same final state. After the fix W19 still reported ~73/200 — all of them
benign.

So all three now force the interleave instead. A GORM callback registered
before the UPDATE soft-deletes the row from a second connection, landing the
delete precisely in the window two racing callers otherwise have to hit by
luck. The defect was never probabilistic; only the scheduling that exposed it
was, and W21's race reproduced at roughly one iteration in a hundred, which
was enough to fail runs on its own.

## A counter is advanced by the database

`nextCode` read the counter, added one in Go and wrote the result back with
no precondition. Two concurrent callers read the same number. On SQLite that
failed outright — each transaction held a read lock and tried to upgrade it,
which deadlocked, and 19 of 30 concurrent `CreateTaskTodo` pairs died with a
raw driver error. On Postgres the second writer would have proceeded once the
first committed, minting two rows sharing one code.

There is no read-modify-write left to guard:

```
INSERT … ON CONFLICT DO NOTHING      -- the counter exists
UPDATE … SET next_number = next_number + 1
SELECT next_number                   -- the number this caller claimed
```

Writing first is what fixes the deadlock: a second caller waits on the write
lock rather than upgrading a read lock it already holds. The increment is the
database's, so nothing is computed from a stale value. The read-back is safe
because the write lock is held until the transaction commits.

The same reasoning would apply to a Postgres sequence, which this does not
use: the counter has to be readable and settable as an ordinary row, because
an import keeps the codes it was exported with and the counter is raised past
them afterwards.

## A failure budget is charged once

`IncrementFailureBudget` was documented as atomic and was not. Five
concurrent charges with five distinct child runs left the counter reading 1.

It now writes under a compare-and-swap on the counter it read, re-reading and
retrying when it loses. Every retry means another caller's increment landed,
so the loop makes progress; the cap at 16 attempts is only there to stop a
pathological stream of concurrent charges spinning forever. The call returns
the value it established rather than re-reading, because the re-read was
itself racy.

Charging the same child twice is still a no-op. That idempotency predates the
fix and has its own test, because a compare-and-swap is exactly the kind of
change that could have cost it.

## A sequence needs one entry point

`RollbackWorkflowRun` documented a four-step compensation: `rolling_back` →
compensate cross-service → compensate own artifacts → `rolled_back`. Nothing
enforced it. Two of the steps were independently-reachable routes, so either
could be called alone, in any order, on a run at any status:

- `DELETE /workflow-runs/{runID}/artifacts` wiped a live run's tasks and
  todos while the run's own status kept reporting `in_progress`.
- `PUT /workflow-runs/{runID}/status` drove a run to the terminal
  `rolled_back` with no compensation having run, leaving tasks still anchored
  to a run reporting itself undone.

Both are closed by deciding who owns the sequence:

- Compensation requires the run to be in `rolling_back`, and answers
  `ErrRollbackNotInProgress` (409) otherwise.
- The three states a rollback drives a run through — `rolling_back`,
  `rolled_back`, `rollback_failed` — are the coordinator's. The public setter
  refuses all three by name; `RollbackWorkflowRun` reaches them through an
  unexported `setRunStatus`.

`WorkflowRunStatus.CanTransitionTo` is deliberately untouched. It describes
which transitions are *legal*, which is a different question from who may ask
for one, and folding the second into the first would have left the
coordinator unable to make the move either.

The consequence for tests: compensation is no longer callable on its own, so
the unit tests that asserted what it does now drive it through
`RollbackWorkflowRun`. A run is only ever *found* in `rolling_back` if a
previous rollback stopped part-way, which is what `ErrRollbackConflict` is
for — the test that covers it writes the row directly to stand in for that
crash, because nothing in the API can produce it any more.
