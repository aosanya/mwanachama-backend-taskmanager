# Narrowing the task list (hub-steward SW19)

`GET /tasks` (`list_tasks`) narrows by `status`, `priority`, `workflow_run_id`
and `held_by`, and since hub-steward SW19 also by the six below. Every filter
that is given must hold; none given lists every task up to the limit.

| Filter | Meaning |
| --- | --- |
| `tag` | A task carries this tag. Repeat it (`?tag=Docs&tag=Blocked`) to require every tag; a comma does not split a name. Over JSON (the MCP tool) it is one name or a list of names. An empty tag is ignored. Names match without regard to case. |
| `repo` | The task's `project_name` is exactly this. |
| `backlog` | The task is in this backlog, given by id or name without regard to case. A task with no stored place, or whose stored backlog id names no backlog, counts as in the first backlog, whose id is `backlog`. An unknown backlog is `ErrUnknownBacklog`, status 400. |
| `step` | The task is at this step, given by id or name without regard to case, of the `backlog` given or of the first backlog when none is given. A task with no stored place, or a stale one, counts as at the step of the first backlog that covers its status, and at no step when none does. An unknown step is `ErrUnknownBacklogStep`, status 400. |
| `untouched_days` | The task has not changed for more than this many days. A negative number is `ErrInvalidFilter`, status 400; 0 or absent does not narrow. |
| `offset` | Skip this many tasks of the filtered list, in rank order, so a list longer than the 500-task page can be read in pages: ask with `offset` 0, 500, 1000 until a page comes back short. A negative number is `ErrInvalidFilter`, status 400; 0 or absent does not skip. |

## Why it is shaped like this

- A tag is matched through a subquery on the tagging table, one per tag, so a
  tag on any number of tasks never becomes a list of bound ids, and a failing
  store read is the call's error, not an empty list.
- `untouched_days` compares `updated_at` with a UTC cutoff, strictly older.
- The hub's copy of this module (`hub-steward`, `wakala-api/modules/taskmanager`)
  carries the same filters under the same names; keep the two in step (D70 of
  the hub-steward-swe repo).

## Rank

`list_tasks` returns tasks in rank order (`rank`, then created time, then id). A
task created with a `rank` above 0 keeps it, so the loader's priority order
survives; a task created without one gets a rank below the bottom of the
list.

The one exception is a completed task (hub-steward SW218). Completed tasks come
after every other task, newest `completed_at` first, with rank, created time and
id as the tie-break, so a Done step read with `step` shows the latest completion
at the top and every other step keeps rank order. `completed_at` is set when a
task reaches a terminal status and is set again whenever the status changes into
one, so a task that returns to Done is placed by its latest completion. There is
no sort control and no setting. `task_pending` lists open tasks only and keeps
rank order.

## What is pending in one call (hub-steward SW67, D87)

`GET /pending-tasks` (`taskmanager_task_pending`) answers what is pending, open or left in one call, so a caller does not page `list_tasks`. Pending is every issue not completed, failed or cancelled that the caller can see. It returns `total`, `by_step` (the count at each step, in step order) and `by_priority`, counted over everything the filters match, and `items`: the first `limit` issues in rank order, each compact (id, title, state, backlog, step, priority, tags, rank, blocked) and without the description. It narrows by `tag` (every tag must match), `backlog`, `step`, `priority` and `q` (every word in the title or a tag, without regard to case); `limit` defaults to 25 and is at most 100, and `offset` pages. It is narrowed by the caller's backlog access exactly as `list_tasks` is; a call with no person is not narrowed. A timing test on 2,000 issues with descriptions requires it to be at least 5 times faster and 20 times smaller than paging `list_tasks`.

This repo has no permission catalog, so the hub's note that `task.read` covers `taskmanager.task.pending` has nothing to port here: a host that maps actions to permissions adds the action itself.

## Tests

`task_pending_test.go` and `routes/pending_tasks_test.go` cover the pending call; `task_list_filter_test.go` covers each filter, their combination, several tags,
case, the 400s and the days boundary; `task_list_tag_rank_test.go` covers
a single-string tag, an empty tag, a caller's rank kept and the list order. `depends_on_traverse_test.go` checks
that `traverse_relationships` returns a task's `depends_on` links outbound and
inbound, and `[]` for a task with none.
