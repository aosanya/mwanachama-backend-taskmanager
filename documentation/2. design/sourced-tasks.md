# Tasks opened from a source (W35)

Something outside this module lists work with dates: a published result
naming what will be done by when, a plan, an agreement. `OpenSourcedTasks`
(`POST /sourced-tasks`, action `taskmanager.sourced_task.open`) turns such a
list into tasks, one per item, without anyone having to remember to.

## Shape

- `source` is an opaque reference the host passes, such as the raw
  `<module>/<instance>/<identity>` of what listed the items. The module stores
  it on the `sourced_task` row and as each task's `linked_ref`, and never
  resolves it.
- Each item has a `title`, an optional `description`, an RFC 3339 `due_at`
  (stored in UTC) and an optional `key`. A keyless item is keyed by its title.
  Two items in one request may not share a key.

## Idempotent per item

`sourced_task`'s primary key is a SHA-256 over the length-prefixed source and
the item key. Replaying a source opens nothing for an item already opened, and
answers that item with its existing task id and `opened: false`; a new item in
the replay is opened. The same key under another source is another item.

## One transaction

Every task and its `sourced_task` row is written in one transaction, and the
`task.created` events are held until it commits, the way a repeating task's
period is made. A request with any bad item is refused whole with
`ErrInvalidSourcedTasks` (400) before anything is written.

## Not here

Who is assigned, and reacting to a source being withdrawn or edited, are the
host's. An item whose due date changes in a later replay keeps the task and the
date it was first opened with.
