# mwanachama-backend-taskmanager — documentation

## Layout

Four folders, in SDLC order, and everything lives under one of them.

| Folder | What's inside |
|--------|---------------|
| [1. requirements/](1.%20requirements/) | Problem, vision and scope for task/workflow management in `mwanachama-frontend-kazi`. |
| [2. design/](2.%20design/) | The task/project/workflow-run graph schema and how it maps onto `mwanachama-backend-shared`'s entity-graph store. |
| [3. implementation/](3.%20implementation/) | The work: `todo.md` (open board), `todo_done.md` (completed rows + board context). |
| [4. qa/](4.%20qa/) | Test coverage and results. |

## Boards and status

| File | What it holds |
| --- | --- |
| [todo.md](3.%20implementation/todo.md) | Open task board |
| [todo_done.md](3.%20implementation/todo_done.md) | Completed rows + board context |

## What this repo is

A Postgres port of `CodeValdWork`'s `TaskManager` — task/project/agent
lifecycle, dependency/blocker tracking, and the `WorkflowRun`
orchestration-and-rollback state machine. Built on
[mwanachama-backend-shared](../mwanachama-backend-shared)'s entity-graph store instead
of ArangoDB, and imported directly by
[mwanachama-backend-api-gateway](../mwanachama-backend-api-gateway) — no gRPC, no
sub-service shape.
