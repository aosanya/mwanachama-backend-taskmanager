# Task states (W31)

Member app decision 278: each template chooses its task states; without a
choice an agency gets **To do, Doing, Waiting, Done, Cancelled**, mapped onto
the eight `TaskStatus` values.

`models.TaskState` is a name and the statuses it covers. `DefaultTaskStates`
is the five:

| State | Statuses |
| --- | --- |
| To do | `pending` |
| Doing | `in_progress` |
| Waiting | `blocked`, `awaiting-direction` |
| Done | `completed` |
| Cancelled | `cancelled` |

`failed` and `split` belong to no default state; a task in either reads with
an empty `state`. Whether they should fold into one of the five is open.

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
unknown field. A status no state covers reads with no state, which is what
`failed` and `split` do by default.

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

## Still open

Whether `failed` and `split` fold into a state, and whether a move may name
a target state rather than a status, are unchanged and undecided.
