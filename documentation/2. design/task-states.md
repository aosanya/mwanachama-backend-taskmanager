# Task states (W31)

Member app decision 278: each template chooses its task states; without a
choice an agency gets **To do, Doing, Waiting, Done, Cancelled**, mapped onto
the eight `TaskStatus` values.

`models.TaskState` is a name and the statuses it covers. `DefaultTaskStates`
is the five:

| State | Statuses |
| --- | --- |
| To do | `pending` |
| Doing | `in_progress`, `split` |
| Waiting | `blocked`, `awaiting-direction`, `failed` |
| Done | `completed` |
| Cancelled | `cancelled` |

`failed` reads as Waiting and `split` as Doing (owner, decision 417,
2026-10-24): a failed task waits on direction to recover, and a split task is
still being worked through its children.

`Task.State` is derived on every read (`CreateTask`, `GetTask`,
`UpdateTask`, `ListTasks`) and is never stored — it carries `spec:"-"`, so
the blueprint has no column for it. The status stays authoritative and every
write still goes through `CanTransitionTo`.

## The setting

The blueprint declares the S64 setting `task_states` (type `json`): an
ordered list of `{name, statuses}`, defaulting to the five above.
`models.ParseTaskStates` reads a value of it and refuses, prefixed
`task states:`, an empty list, a state with no name or declared twice, a
status that is not one of the eight, a status mapped by two states, and an
unknown field. A status no state covers reads with no state; the default five
cover all eight.

`NewTaskManager` checks that the blueprint default parses and fails
construction when it does not.

## The stored layers

An agency's two layers live in the settings tables `spec.Migrate` provisions
in the module's own table set (shared's S64:
`spec_settings_definition`, `spec_settings_override`,
`spec_settings_change`). Every read that answers `state` — `CreateTask`,
`GetTask`, `UpdateTask`, `ListTasks`, `ListOverdueTasks` — resolves
`task_states` through `spec.ResolveStored`, module default then the
definition then the owner's override, and runs `ParseTaskStates` on the
result. The value is resolved per call, not cached in the manager, so an
override takes effect on the next read without rebuilding the manager. A
stored value that does not parse fails the read with the `task states:`
error rather than falling back to the five.

A host writes the layers through four package functions, each holding the
value to `ParseTaskStates` before anything is stored:

| Function | Does |
| --- | --- |
| `StoredTaskStates(db, spec)` | the effective list, as reads see it |
| `DefineTaskStates(db, spec, value)` | sets the template's definition, keeping every other defined setting (`PutDefinitionSettings` replaces the whole layer, so the current definition is read and written back with `task_states` set) |
| `OverrideTaskStates(db, spec, value, by, at)` | sets the owner's override; the change is recorded by the engine |
| `ClearTaskStatesOverride(db, spec, by, at)` | removes the override, falling back to the definition; recorded too |

`by` and `at` are the host's: the module does not know who the owner is.
They are package functions rather than `TaskManager` methods and are not
declared in `taskmanager.operations.json`; whether an override becomes an
address, and who may call it, is the host's to decide.

## Reading the states

`GET /task-states` (`ListTaskStates`, action `taskmanager.task_state.list`)
answers `{"items": [...]}`: the agency's effective states in their declared
order, each with the statuses it covers, resolved the same way a task read
resolves them. It is what a board draws its columns from.

## Moving a task by state

`POST /tasks/{taskID}/move` with `{"state": "<name>"}` (`MoveTaskToState`,
action `taskmanager.task.move`; decision 417). The status stays
authoritative, so a move is a walk over `CanTransitionTo`:

- A task whose status is already in the named state is returned unchanged.
- Otherwise `StatusPathToState` searches breadth-first from the current
  status and stops at the shallowest depth that reaches a status of the
  state; when several are reached at that depth, the one listed first in the
  state wins. To do → Done is `in_progress`, `completed`; Doing → Waiting is
  `awaiting-direction`.
- Each step is written through `UpdateTask`, so every rule an ordinary status
  change meets still applies — a blocked pending task refused on the way to
  `in_progress` answers `ErrBlocked` (409), and the timestamps, events and
  case close-out each step triggers happen as they would by hand. The steps
  are not one transaction: a refusal part-way leaves the task at the last
  status it legally reached.
- A name the agency's states do not hold is `ErrUnknownTaskState` (400).
- A state no legal path reaches — anything out of `completed` or
  `cancelled` — is `ErrNoPathToState` (409), and the message names the
  current status and the state it could not reach.

`UpdateTask` with a status is unchanged and still works.
