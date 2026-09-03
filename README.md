# mwanachama-backend-taskmanager

Postgres port of `CodeValdWork`'s `TaskManager` — task/project/agent
lifecycle, dependency and blocker tracking, and the `WorkflowRun`
orchestration/rollback state machine, for
[mwanachama-frontend-kazi](../mwanachama-frontend-kazi).

No gRPC, no sub-service shape. Built on
[mwanachama-backend-shared](../mwanachama-backend-shared)'s Postgres entity-graph store
and imported directly by
[mwanachama-backend-api-gateway](../mwanachama-backend-api-gateway).

See [documentation/](documentation/) for design and task board.
