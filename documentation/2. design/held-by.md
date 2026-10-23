# Work held outside the board (W36)

A task can wait on somebody who is not on the board at all: a service
company, a supplier, another body. Before W36 such a task was simply
`blocked`, which reads the same as one blocked by a task on the board, so a
board could not tell its own queue from work another party is holding.

## The field

`task.held_by` is a plain string: **who outside the board holds a blocked
task**. It is an opaque label or reference, such as a provider's name or a
raw key another module owns. The module stores it and reads it back; it never
resolves it and names no kind of holder, so the same field serves any domain.

- **It is set on a move to `blocked`** (or while staying there) through
  `UpdateTask`, with the rest of the task's mutable fields.
- **It lives only while the task is `blocked`.** Any write that leaves the
  task in another status clears it: `UpdateTask` empties it before the write,
  and the internal `setTaskStatus` (the assignment gate and
  `UnblockDependents`) writes `held_by = ''` with every status other than
  `blocked`. A pending task sent with a holder is stored with none.
- **Empty means nothing outside holds it**: either not blocked, or blocked by
  something on the board (a `blocks` relationship, an unmet dependency, an
  operator's hold with only a `blocker_note`).

`blocker_note` stays what it was, the prose reason; `held_by` is the holder.

## Reading it

Every task read carries `held_by` when it is set. `GET /tasks?held_by=<x>`
(`TaskFilter.HeldBy`) lists the blocked work one holder has, backed by the
`held_by` index. A board telling outside work apart reads its blocked listing
and splits on whether `held_by` is empty.

## Not built

- No "held by anyone outside" filter (`held_by` non-empty): a board can split
  a blocked listing itself, and an exact match covers one holder's queue.
- No history of who held a task. The assignment history (`assignees.md`)
  records the inside worker only.
- No check that the holder is a known party; the host decides what a holder
  string means.
