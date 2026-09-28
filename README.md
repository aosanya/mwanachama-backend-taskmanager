# mwanachama-backend-taskmanager

Work, declared rather than written: tasks and the steps they decompose into,
who holds them, what blocks what, and the run that anchors a whole
orchestrated execution and can undo it.

A Go library backed by Postgres through GORM. There is no service to run —
`spec.Migrate(db, s)` creates the tables and `NewTaskManager(db, s, pub)`
returns the manager the rest of your code calls.

## Objects are declared, not written

The tables come from a JSON spec. `taskmanager.blueprint.json` declares the
module's fourteen objects once; a domain spec names them in its own words and
nothing else. The engine is `mwanachama-backend-shared/spec`, `specstore` and
`dispatch`.

```go
s, err := taskmanager.SpecFor("wakala")   // or LoadSpec(path) for a spec of your own
err = taskmanager.Provision(db, s)
tm, err := taskmanager.NewTaskManager(db, s, publisher)
```

Two domains ship, and `TestShippedExamplesCoexist` migrates both into one
database:

| | `spec/examples/work.taskmanager.json` | `spec/examples/agency.taskmanager.json` |
| --- | --- | --- |
| instance | `kazi` | `wakala` |
| a task | `task` | `work_item` |
| a step | `task_todo` | `step` |
| a worker | `agent` | `operator` |
| a container | `project` | `objective` |

See [documentation/2. design/declared-objects.md](documentation/2.%20design/declared-objects.md).

## Routes are declared too

`taskmanager.operations.json` declares sixty addresses;
`mwanachama-backend-shared/dispatch` builds the handlers and
`routes/routes.go` is 130 lines of adapter. Adding an address is an edit to
that file rather than a new Go function.

Nothing is anonymous: a work board has no public half, so every route arrives
gated and a mount wraps its own capability check around it. This module has
no auth model, matching `mwanachama-backend-catalog`.

See [documentation/2. design/routes.md](documentation/2.%20design/routes.md).

## Mounted per agency

`mwanachama-wakala-api` gives each registered agency its own board: one
instance per agency, one table set per instance, resolved per request off the
URL's `{instanceID}` under `/instances/{instanceID}/taskmanager`. `Shape()`
is what makes that possible — the declared table with no handlers, in
`Dispatch`'s own order.

## What it enforces

- Every task lands `pending`, whatever status the caller asked for, and its
  id is server-minted.
- A status change is checked against the lifecycle state machine, and
  `pending → in_progress` is refused while any non-terminal task blocks it.
- A run's rollback resets the tasks it produced and soft-deletes its steps,
  and is refused while another run still depends on one of them.
- A recovery run is charged against its lineage's shared budget, and
  charging the same child twice is a no-op.
- A stable `Code` is minted inside the same transaction as the row it
  numbers.

## Status

Converted to the spec-driven shape on 2026-09-28: the blueprint, both domain
specs, the store on `specstore`, the sixty declared operations, and the
wakala-api mount. `gormstore/` and the thirteen hand-written route builders
are gone.

Open bugs are on
[documentation/3. implementation/todo.md](documentation/3.%20implementation/todo.md).
W14, W15, W19, W20, W21 and W22 are pre-existing and survived the conversion
untouched — each still has its pinning test, and W21's was made deterministic
rather than left to a one-in-a-hundred race.

## Test

```sh
go test ./...        # in-memory SQLite, no database needed
```

`postgres_integration_test.go` is `//go:build integration` and gated on
`POSTGRES_URL`; it is not part of `go test ./...`.

## Licence

Apache-2.0.
