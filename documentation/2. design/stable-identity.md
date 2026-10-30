# A thing's stable identity

`TaskManager` embeds the shared `identity.Owner` (shared S56), so a host that resolves a manager per instance already holds the owner for it. This is board row W27.

## The raw

`StableIdentity(ctx, object, id)` answers `taskmanager/<instance>/<uuid>` for a row of one of eight objects: `task`, `project`, `agent`, `case`, `repeating_task`, `task_todo`, `workflow_run` and `acceptance_criteria`. The instance is the spec's instance, the one that prefixes the tables. The identity is the row's own id: nothing here re-mints ids (a task keeps its id for life), so unlike agency's Code there is nothing to survive a fork.

The raw carries no object name, so `Resolve(ctx, raw)` tries the objects in the order above and answers the first row holding that id, as `(object, id)`. Ids are server-minted uuids, so two objects never share one in practice.

A row soft-deleted (`deleted = true` on a task, project or step) is a miss, the same as a read of it, so a consumer's staleness handling sees a confirmed `ErrNotFound`.

## What it answers

- `identity.ErrInvalidRaw` for a string that does not parse;
- `identity.ErrNotOwned` for another module's raw or another instance's;
- `identity.ErrNotFound` for an id no identity object holds, or a deleted one;
- `ErrNoStableIdentity` from `StableIdentity` for an object with no identity of its own: a blocker, dependency, membership, tagging, case task, case vote, the generated, sourced and period rows, a tag, a deliverable, an import job, an assignment.

An import job is a process that ends, a tag is found by its name and a deliverable has no operation naming it, so none of the three is an identity.

## The `refers` marks

Every path or body argument that names one of the eight objects carries `"refers": "taskmanager.<object>"` in `taskmanager.operations.json` (shared S57), so `Spec.References()` lists them for a consumer to resolve:

| Argument | Refers to |
|---|---|
| `taskID` | `taskmanager.task` |
| `projectID` | `taskmanager.project` |
| `idOrSlug`, `agent_id` | `taskmanager.agent` |
| `caseID` | `taskmanager.case` |
| `repeatingTaskID` | `taskmanager.repeating_task` |
| `todoID` | `taskmanager.task_todo` |
| `criteriaID` | `taskmanager.acceptance_criteria` |
| `runID`, `rootRunID`, `workflow_run_id`, `parent_run_id`, `root_run_id`, `child_run_id` | `taskmanager.workflow_run` |

Left unmarked on purpose: `agentID` (an external slug only), `taskName` and `projectName` (names), `jobID`, `taskOrTodoID` (two objects), `assignee_id` (its kind is a sibling argument), the member and requester arguments (another module's ids), the relationship vertices (any object), and every `whole` body or query, which `refers` cannot mark.

`idOrSlug` and `agent_id` accept the row id or the external slug. A slug handed to `StableIdentity` is `ErrNotFound`: the raw is only ever the uuid.

## The conversation

A tool's `Handle` takes the MCP `_meta` (shared S58) and puts `conversation_id` on the context the manager sees. This repo registers no MCP server, so there is no adapter to change; `routes/tool_conversation_meta_test.go` pins that `routes.ToolsFor` passes it through, and a host that still calls `Tool.Invoke` passes none.

Tests: `identity_owner_test.go`, `operations_refers_test.go`, `routes/tool_conversation_meta_test.go`.
