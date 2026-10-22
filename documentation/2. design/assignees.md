# Assignees: a worker, a person or a role (W28)

Member app decisions 272 and 275 (2026-10-22): a task is assigned to a
person or a role, and whoever holds the role picks it up. An AI agent stays
one kind of assignee.

## What a task carries

`task.assigned_to` still holds the assignee's id. `task.assignee_kind` says
how to read it:

| `assignee_kind` | `assigned_to` is | Who resolves it |
| --- | --- | --- |
| `agent` | an `agent` row's id | this module (`AssignTask`, unchanged address) |
| `actor` | the host's id for a person | the host |
| `role` | the host's id for a role kind | the host |
| empty | nothing, or a row written before the field existed (an agent) | — |

`assignee_kind` is a plain string on the task, not an enum, because an
unassigned task carries none and every older row was written without one.
The closed set lives on the `assignment` object, where every row has a kind.

`AssignTaskToHolder(task, kind, id)` (`POST /tasks/{taskID}/assign-holder`)
accepts only `actor` and `role`; a worker is still handed a task through
`AssignTask`, which finds or validates the `agent` row and dispatches
`task.assigned`. A person or a role is not dispatched to: telling them is
the host's (wakala-api WK182).

## A role's tasks, for each holder

**This module does not know who holds a role.** That is the host's
knowledge (actor's role assignments on wakala-api). `ListHeldTasks` takes a
`HolderTasksQuery{ActorID, RoleKindIDs}` that the host fills after resolving
which role kinds the caller holds, and returns the tasks assigned to that
person plus the tasks assigned to any of those roles. A person never picks
up a role's task by sharing its id: the kind is matched together with the
id. An empty query lists nothing.

It is deliberately **not a declared address**. A body naming its own role
kind ids would let any caller claim any role; the host must resolve them
from the session and call the manager, the same way forms binds a member
from the caller.

## History survives unassigning

Every hand-over writes an `assignment` row, a stretch from `assigned_at`
until `unassigned_at`. Handing a task to somebody else closes the current
stretch and opens a new one; `UnassignTask` closes it and clears the task's
two columns. Rows are never deleted. `ListTaskAssignments`
(`GET /tasks/{taskID}/assignments`) reads them oldest first.

## Known edges

- `UnblockDependents` re-fires `task.assigned` only for an agent; for a
  person or a role its `GetAgent` lookup misses and it logs and moves on,
  which leaves the task released but not dispatched. That is the intended
  outcome (nothing to dispatch to), reached by an existing log path.
- Which assignees the studio offers (decision 275: owners, agents and roles
  held by staff) is the host's filter, not this module's.
