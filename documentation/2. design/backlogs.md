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

Each action is its own permission, so a role can be given listing without
managing. `taskmanager.backlog.manage` is the one the phone app checks to show
its Add and Edit backlog screens. Moving a task to a step has no route yet;
it is a manager method only, as in the hub's copy.

## Not here yet

- Access by group (a group's Can see or Can move on a backlog) is hub-steward
  SW32 and is ported separately.
- The `backlog` and `step` filters on `list_tasks` are hub-steward SW19, ported
  separately.
- `TestNewCardsLandAtTheBottomInTheOrderTheyWereCreated` and the other
  `task_rank_test.go` tests fail on this repo's HEAD before this change; they
  are unrelated to backlogs.
