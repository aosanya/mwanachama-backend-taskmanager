# Narrowing the task list (hub-steward SW19)

`GET /tasks` (`list_tasks`) narrows by `status`, `priority`, `workflow_run_id`
and `held_by`, and since hub-steward SW19 also by the five below. Every filter
that is given must hold; none given lists every task up to the limit.

| Filter | Meaning |
| --- | --- |
| `tag` | A task carries this tag. Repeat it (`?tag=Docs&tag=Blocked`) to require every tag. Names match without regard to case. |
| `repo` | The task's `project_name` is exactly this. |
| `backlog` | The task is in this backlog. Only `default` exists until the backlogs port lands; any other name is `ErrUnknownBacklog`, status 400. |
| `step` | The task is at the step of this name, matched without regard to case. Until the backlogs port lands the steps are the task states; an unknown name is `ErrUnknownTaskState`, status 400. |
| `untouched_days` | The task has not changed for more than this many days. A negative number is `ErrInvalidFilter`, status 400; 0 or absent does not narrow. |

## Why it is shaped like this

- A tag is matched through a subquery on the tagging table, one per tag, so a
  tag on any number of tasks never becomes a list of bound ids, and a failing
  store read is the call's error, not an empty list.
- `untouched_days` compares `updated_at` with a UTC cutoff, strictly older.
- The hub's copy of this module (`hub-steward`, `wakala-api/modules/taskmanager`)
  carries the same filters under the same names; keep the two in step (D70 of
  the hub-steward-swe repo).

## Tests

`task_list_filter_test.go` covers each filter, their combination, several tags,
case, the 400s and the days boundary. `depends_on_traverse_test.go` checks
that `traverse_relationships` returns a task's `depends_on` links outbound and
inbound, and `[]` for a task with none.
