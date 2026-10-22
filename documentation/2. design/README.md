# Design

| Page | What it covers |
| --- | --- |
| [declared-objects.md](declared-objects.md) | The fourteen objects declared in `taskmanager.blueprint.json`, the two shipped domains, and what the conversion changed in the Go types. |
| [routes.md](routes.md) | The sixty operations declared in `taskmanager.operations.json`, how a mount gates them, and the four addresses that changed. |
| [concurrency-and-sequence.md](concurrency-and-sequence.md) | Why a write re-checks what its read assumed, why the code counter is advanced by the database, and why a rollback has one entry point. |
| [repeating-tasks.md](repeating-tasks.md) | A repeating task's template, the period it is due in, the key that makes one task per period, and what is left to build (W72). |

Both pages describe the shape this repo took on 2026-09-28, when it moved off
hand-written row structs and hand-written handlers onto
`mwanachama-backend-shared`'s spec engine — the same shape
`mwanachama-backend-catalog` and `mwanachama-backend-agency` already run.
