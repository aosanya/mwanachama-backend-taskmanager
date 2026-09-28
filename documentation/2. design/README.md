# Design

| Page | What it covers |
| --- | --- |
| [declared-objects.md](declared-objects.md) | The fourteen objects declared in `taskmanager.blueprint.json`, the two shipped domains, and what the conversion changed in the Go types. |
| [routes.md](routes.md) | The sixty operations declared in `taskmanager.operations.json`, how a mount gates them, and the four addresses that changed. |

Both pages describe the shape this repo took on 2026-09-28, when it moved off
hand-written row structs and hand-written handlers onto
`mwanachama-backend-shared`'s spec engine — the same shape
`mwanachama-backend-catalog` and `mwanachama-backend-agency` already run.
