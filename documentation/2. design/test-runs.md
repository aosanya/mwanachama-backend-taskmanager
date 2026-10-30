# A board for one test run (HS280)

`ForRun(runID)` returns a `TaskManager` that reaches every table through a store scoped to the run. It reads and writes the `_test` copy of each table, stamps `run_id` on every row it inserts, and reads only rows with that run id, so two runs using the same key never meet and no row reaches a live table. The parent is not changed. An empty run id is refused with `specstore.ErrRunIDEmpty`. The host removes a run's rows with the shared engine's `RemoveRun` and lists them with `RowsOfRun`.

## What goes through the run

- Every read and write by role goes through the store, which adds the run filter.
- The relationship edges (the foreign-key columns and the join tables for blocks, depends on, member of and has tag) go through `onTable`, which adds the same filter, and the join-table rows are stamped with the run id.
- The batch operations (generated tasks, sourced tasks, a repeating task's period) open a transaction manager with `inTx`, which keeps the run, the spec and the overdue grace of the parent.
- Task state names are read from the live settings; a run reads them and does not change them.

## Tag names are read in two steps

The run filter is written against the bare `run_id` column. A join between the tagging and tag tables names it in both, so the database refuses it as ambiguous and the read silently returned no tags. Tag names are now read as the tag ids of the task first, then the names of those tags.

## The spec and its test copy

The board spec loads under the instance id wakala-api mints (`agy` and eight hex characters) with the mount `board`. The longest test-copy index name is 62 bytes, one under what Postgres truncates at. `testrun_test.go` measures every table and index, live and test copy, and creates the test copy.
