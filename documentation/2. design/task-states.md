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

## Not built

Where an agency declares its own states. W32 names S64 module settings as
the place; until that lands `taskManager.taskStates` answers the default five
for every instance. When it does, it is the one function to change.
