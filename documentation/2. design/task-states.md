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

`NewTaskManager` resolves the setting once through `spec.Resolve` and keeps
the result; a blueprint default that does not parse fails construction.

## Not built

The definition layer (what an agency's template sets) and the owner's
override layer are not passed in yet: S64 has not settled where an agency's
two layers are stored. Until it does, `moduleTaskStates` resolves with no
definition and no override, so every instance reads the module default.
When storage lands, those two layers are its only change.
