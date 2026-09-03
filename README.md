# mwanachama-taskmanager

Postgres port of `CodeValdWork`'s `TaskManager` — task/project/agent
lifecycle, dependency and blocker tracking, and the `WorkflowRun`
orchestration/rollback state machine, for
[mwanachama-frontend-kazi](../mwanachama-frontend-kazi).

No gRPC, no sub-service shape. Built on
[mwanachama-go-shared](../mwanachama-go-shared)'s Postgres entity-graph store
and imported directly by
[mwanachama-api-gateway](../mwanachama-api-gateway).

See [documentation/](documentation/) for design and task board.
