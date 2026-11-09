# Backlogs and steps (hub-steward SW15, SW20, SW26)

A hub keeps **backlogs**. A backlog is a named, ordered list of **steps**, and a
step does not have to be one of the eight lifecycle statuses. A task sits in
exactly one backlog at one step. This is the same change the hub's copy under
`projects/hub-steward/code/wakala-api/modules/taskmanager` carries (hub-steward
commits 5b07345 and 83fdfa8); the owner approved making it in both copies
(hub-steward-swe decision D70).

## What is stored

| Where | What |
| --- | --- |
| Setting `backlogs` (type `json`) | The ordered backlogs, each `{id, name, steps: [{id, name, statuses}]}`. The default is one backlog, `backlog`, with the five states of `task_states`: To do, Doing, Waiting, Done, Cancelled. |
| Task fields `backlog` and `step` | The ids of the backlog and the step the task sits at. Empty reads as the first backlog at the first step that covers the task's status. |

`models/backlog.go` holds the types and the lookups; `backlog.go` holds the
manager methods. `Task.Backlog` and `Task.Step` are filled on every read
(`GetTask`, `ListTasks`) and are checked on `CreateTask`; `UpdateTask` keeps
the stored place, so changing a task's fields never moves it.

## What the manager does

- `ListBacklogs` returns each backlog with its steps and the number of issues
  at each.
- `CreateBacklog`, `RenameBacklog`, `DeleteBacklog`, `AddBacklogStep`,
  `RenameBacklogStep`, `ReorderBacklogSteps`, `RemoveBacklogStep` change one
  backlog; `ReplaceBacklogs` sets the whole list and always keeps at least one
  backlog.
- `MoveTaskToStep` puts a task at a step of any backlog. When the step names
  lifecycle statuses the task walks its status there through the same
  transition rules as `MoveTaskToState`; a step that names none leaves the
  status alone.
- A placed task also moves to the step that names its new status when the status
  changes outside `UpdateTask`: blocking on assign, unblocking dependents,
  cancelling a run or a case, and rolling a run back. An unplaced task is not
  pinned by these paths; it keeps reading from its status.
- A step that holds issues cannot be removed, and a backlog that holds issues
  cannot be deleted: both return `ErrBacklogHoldsIssues` (409), including when
  `ReplaceBacklogs` would drop them. `ErrUnknownBacklogStep` and
  `ErrInvalidBacklog` answer 400.

## The operations

| Operation | Route | Action |
| --- | --- | --- |
| `list_backlogs` | `GET /backlogs` | `taskmanager.backlog.list` |
| `create_backlog` | `POST /backlogs` | `taskmanager.backlog.create` |
| `rename_backlog` | `PUT /backlogs/{backlogID}` | `taskmanager.backlog.rename` |
| `delete_backlog` | `DELETE /backlogs/{backlogID}` | `taskmanager.backlog.delete` |
| `replace_backlogs` | `PUT /backlogs` | `taskmanager.backlog.manage` |
| `add_backlog_step` | `POST /backlogs/{backlogID}/steps` | `taskmanager.backlog_step.add` |
| `rename_backlog_step` | `PUT /backlogs/{backlogID}/steps/{stepID}` | `taskmanager.backlog_step.rename` |
| `remove_backlog_step` | `DELETE /backlogs/{backlogID}/steps/{stepID}` | `taskmanager.backlog_step.remove` |
| `reorder_backlog_steps` | `PUT /backlogs/{backlogID}/step-order` | `taskmanager.backlog_step.reorder` |
| `move_task_to_step` | `POST /tasks/{taskID}/move-to-step` | `taskmanager.task.move_to_step` |

Each action is its own permission, so a role can be given listing without
managing. `taskmanager.backlog.manage` is the one the phone app checks to show
its Add and Edit backlog screens. Moving a task to a step is the tenth
operation: it names the backlog and the step by id.

## Access by group

A group has Can see or Can move on a backlog, and one group, the owner group
(the Product owner group), always has Can move on every backlog. The grants
live in the `backlog_access` setting and the owner group in
`backlog_owner_group`; both are stored per hub like the `backlogs` setting.

| Rule | Where |
| --- | --- |
| While no grant and no owner group is set, every backlog is open to everyone. Once either is set, a person sees only the backlogs one of their groups has a grant on, or all of them through the owner group. | `AccessLevelOf` in `models/backlog_access.go` |
| A person's level is the highest any of their groups gives on that backlog. | `AccessLevelOf` |
| A new backlog starts seen by the owner group alone, because only it has a grant until someone gives more. | `CreateBacklog` adds no grant |
| The first `give_backlog_access` is refused with 409 (`ErrOwnerGroupNotSet`) until `set_backlog_owner_group` has named a group, so a restricted hub cannot hide every backlog from its managers. | `GiveBacklogAccess` |
| `set_backlog_owner_group` refuses an empty or unknown group, and refuses when the hub's groups cannot be looked up. | `SetBacklogOwnerGroup` |
| Reads: list_tasks, list_backlogs, read one task, read by name, a task's assignments, a project's, a run's and a case's tasks, the overdue list and what a task waits on show a person only issues in backlogs they can see. A hidden issue answers not found; edges to hidden issues are left out of the reply. | `readScope`, `narrowToVisible` |
| Writes: create (in the backlog it lands in, the first when none is named), update (and the backlog it is updated into), move to a state, place, delete and move to a step need Can move; an edge needs Can move on the issue it starts from and Can see on the one it points to. The other writes on an issue need Can move on its backlog too: assign, unassign, hand to a holder, unblock, unblock dependents, add to or remove from a project, create in a project (in the backlog it lands in), link to a run or a case, and cancel or roll back a run or cancel a case, which need it on every issue of the run or case. A person with Can see gets 403 (`ErrBacklogForbidden`). Opening tasks from a source and generating tasks create issues in the first backlog, so they need Can move on it; a call with no person or the owner group is not narrowed. Stopping a run, a run's step calls, timeouts and ending a case as done are the run engine's and are not checked. | `backlog_access_writes.go`, `backlog_access_writes_more.go`, `backlog_access_sourced.go` |
| A call with no person (a loader, an agent) is not narrowed. | `requester_id` empty |
| On a hub with an owner group or grants, a person whose groups cannot be looked up, or a module with no lookup set, sees and writes nothing. | `backlogVisibility` |
| `give` and `take` run in one transaction, so eight groups given access at once all keep their grant. A grant to a group that no longer exists is listed with `gone` and counts for nothing. Grants of a deleted backlog are dropped when the backlogs are saved. | `changeGrants`, `pruneGrants` |
| Saving the `backlogs` setting, giving or taking a group's access and pruning the grants of a deleted backlog all take the same package mutex and, on Postgres, the same transaction-level advisory lock keyed on the task table and the setting, so two simultaneous edits both survive and no grant is lost. The group role sync after a change runs outside the lock. | `saveBacklogs`, `changeGrants`, `pruneGrants`, `lockBacklogs` |
| An issue that names a backlog that no longer exists reads in the first backlog and stays visible to whoever sees it. | `narrowToVisible` |

The module does not know about groups itself. The hub that mounts it hands in
two small functions: `UseGroups` returns the group ids of a person, and
`UseGroupExists` says whether a group id exists. The hub steward registry wires
its actor groups to them. A hub that wires neither gets the closed behaviour in
the table, so a restricted hub never falls open.

`requester_id` comes from the session (`"from": "caller"` in the operations
file), never from the request body or the query, so a caller cannot ask as
someone else.

The four operations:

| Operation | Route | Action |
| --- | --- | --- |
| `list_backlog_access` | `GET /backlogs/{backlogID}/access` | `taskmanager.backlog_access.list` |
| `give_backlog_access` | `PUT /backlogs/{backlogID}/access/{groupID}` | `taskmanager.backlog_access.give` |
| `take_backlog_access` | `DELETE /backlogs/{backlogID}/access/{groupID}` | `taskmanager.backlog_access.take` |
| `set_backlog_owner_group` | `PUT /backlog-owner-group` | `taskmanager.backlog.set_owner_group` |

## Roles of a group

The module has no roles and no permission catalog. The hub that mounts it may
hand in `UseRoleSync`, an object with two methods: `SetBacklogRole` receives a
group id and the role code the group should now hold (`backlog_member` for Can
see, `backlog_worker` for Can move, empty for none), and `GroupRoleState`
reports the role codes a group holds and whether they allow reading and
editing. Giving or taking access, and dropping the grants of a deleted
backlog, call `SetBacklogRole` with the group's highest level over all
backlogs (`HighestLevelOf`); the owner group is never given a role. The access
list then carries `roles`, `no_role` (the roles do not allow reading) and
`role_too_small` (Can move with roles that do not allow editing) for each
group. With no hook set nothing is synced and those fields stay empty. Group
roles themselves and the admission they give are the hub's.
