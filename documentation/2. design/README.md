# Design

| Page | What it covers |
| --- | --- |
| [declared-objects.md](declared-objects.md) | The fourteen objects declared in `taskmanager.blueprint.json`, the two shipped domains, and what the conversion changed in the Go types. |
| [routes.md](routes.md) | The sixty operations declared in `taskmanager.operations.json`, how a mount gates them, and the four addresses that changed. |
| [concurrency-and-sequence.md](concurrency-and-sequence.md) | Why a write re-checks what its read assumed, why the code counter is advanced by the database, and why a rollback has one entry point. |
| [held-by.md](held-by.md) | `held_by`: who outside the board holds a blocked task, cleared when it leaves blocked, and the `held_by` filter (W36). |
| [repeating-tasks.md](repeating-tasks.md) | A repeating task's template, the period it is due in, the key that makes one task per period, and what is left to build (W72). |
| [run-versions.md](run-versions.md) | A run keeps the workflow version it began on, the one partial index on runs still going, and Stop with a start again on the good version (HS300). |
| [backlogs.md](backlogs.md) | Backlogs with their own ordered steps, a task in one backlog at one step, the nine backlog operations and their actions, and why a step or backlog holding issues is refused (hub-steward SW15, SW20, SW26). |
| [list-filters.md](list-filters.md) | The five filters `list_tasks` gained for the hub-steward backlog (tag, repo, backlog, step, untouched_days), their rules and why a tag is a subquery (hub-steward SW19). |

Both pages describe the shape this repo took on 2026-09-28, when it moved off
hand-written row structs and hand-written handlers onto
`mwanachama-backend-shared`'s spec engine — the same shape
`mwanachama-backend-catalog` and `mwanachama-backend-agency` already run.
