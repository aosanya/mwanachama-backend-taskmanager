# Hub copy diff

`mwanachama-backend-taskmanager` is the custodian of the task manager module. The hub (`projects/hub-steward/code/wakala-api/modules/taskmanager`) holds a copy that was changed after it was made. Every change made in that copy exists here, so the two differ by the import path only. Hub issue W72 moved the changes over in fifteen groups, one commit each, each naming the hub commits it carries.

## Command

```
scripts/hub-copy-diff.sh [path-to-hub-modules-taskmanager]
```

The default path is `../projects/hub-steward/code/wakala-api/modules/taskmanager`, relative to this repository's parent folder. The exit code is 1 when the trees differ, 0 when they do not. The script is the one of `mwanachama-backend-shared`, generalised: the module path and the folder name come from `go.mod`, and the whole module is compared instead of chosen packages.

Compare against an export of the hub's committed branch, not its working tree. The hub checkout is edited by other sessions, and a file they have not committed yet (on 2026-11-16 `task_due.go`) shows as `Only in hub`:

```
git -C ../projects/hub-steward archive dev code/wakala-api/modules/taskmanager | tar -xf - -C /some/empty/folder
scripts/hub-copy-diff.sh /some/empty/folder/code/wakala-api/modules/taskmanager
```

## What it does

1. Copies the `*.go` and `*.json` files of both trees into two temporary folders, leaving out `cmd/`, `documentation/`, `scripts/` and `.git/`.
2. Rewrites the hub's imports: `wakalaserver/modules/shared/` becomes `github.com/aosanya/mwanachama-backend-shared/`, and `wakalaserver/modules/taskmanager` becomes this module's path.
3. Runs every Go file on both sides through `scripts/hubdiff`, which parses it, drops all comments and prints it with `gofmt`, so comments and layout do not show as differences.
4. Runs `diff -r backend hub`.

## Result of 2026-11-16

Run at the commit that adds this page (the fifteen group commits plus the last alignment of `errors.go`, `overdue.go`, `task.go`, `task_list_filter.go` and the operations file to the hub's order and wording) against an export of the hub's `dev` branch at 169958ee, whose last commit that touches the module is 5063fbe7. The exit code is 1 and the output is 71 lines, all `Only in backend`. Every file the hub has, Go, blueprint and operations, is identical after the rewrite, and nothing is `Only in hub`.

## The `Only in backend` lines

All are files written here that the hub copy never carried.

| Files | Reason |
|---|---|
| `backend_testdb_test.go`, `spechelper_test.go`, `routes/spechelper_test.go` | Test helpers only this repository uses: managers over a publisher and over tables (`newTestManagerWithPublisher`, `newTestManagerWithTables`, `specAndTablesForInstance`) and the route helper for its `routes_test.go`. The hub's own helpers live in `testdb_test.go`. |
| `spec/examples/work.taskmanager.json` | The example the repository's tests migrate beside the hub's `agency.taskmanager.json`. It carries the same objects (`task_caps` and `task_usage` were added with the caps group) under the `work` domain and the `kazi` instance. |
| `postgres_integration_test.go`, `fake_test.go`, `testrun_test.go`, `blueprint_test.go`, `operations_refers_test.go`, `identity_owner_test.go` | Tests of the module as a library: a real Postgres, the fake publisher, a run's copy of the board, the blueprint's own shape, the `refers` of every operation and the stable identity of each object. |
| `agent_test.go`, `assignee_test.go`, `assignee_notice_test.go`, `assignment_unblock_test.go`, `blocker_test.go`, `held_by_test.go`, `linked_ref_test.go` | The first suites of assignment, blockers and holders, written here before the hub copy was made. |
| `case_test.go`, `case_status_paths_test.go`, `case_vote_test.go`, `routes/case_end_test.go`, `routes/case_vote_test.go` | Cases and their votes. |
| `codesequence_test.go`, `todo_code_test.go` | The counters behind todo and tag codes, including the two concurrent-create races. |
| `events_test.go`, `overdue_test.go`, `project_test.go`, `relationship_test.go`, `relationship_json_test.go`, `tag_test.go`, `task_test.go` | Event payloads, overdue reads, projects, relationships, tags and tasks as a library. |
| `generated_test.go`, `sourced_test.go`, `routes/generated_task_test.go` | Generated and sourced tasks. |
| `repeating_dates_test.go`, `repeating_store_test.go`, `repeating_test.go`, `repeating_tx_test.go`, `repeating_wire_test.go`, `routes/repeating_task_test.go` | Repeating tasks. |
| `task_list_offset_test.go`, `task_list_tag_rank_test.go`, `routes/list_tasks_offset_test.go` | List paging, tag and rank order. |
| `task_state_path_test.go`, `task_state_setting_test.go`, `task_state_stored_test.go`, `task_state_test.go`, `routes/task_state_move_test.go` | Task states and the move between them. |
| `workflow_run_cancel_test.go`, `workflow_run_closed_test.go`, `workflow_run_failure_budget_test.go`, `workflow_run_id_test.go`, `workflow_run_rollback_test.go`, `workflow_run_step_limit_test.go`, `workflow_run_test.go`, `workflow_run_version_test.go`, `routes/workflow_run_closed_test.go`, `routes/workflow_run_step_limit_test.go`, `routes/workflow_run_stop_test.go` | The workflow run lifecycle. |
| `routes/routes_test.go`, `routes/tools_test.go`, `routes/tool_conversation_meta_test.go` | The route table and the tool descriptions. |
| `routes/w11_caller_supplied_id_test.go`, `routes/w12_last_event_at_watchdog_evasion_test.go`, `routes/w14_rollback_sequence_bypass_test.go`, `routes/w15_failure_budget_increment_race_test.go`, `routes/w16_import_silent_dependency_drop_test.go`, `routes/w17_import_duplicate_shortkey_test.go`, `routes/w19_task_update_delete_race_test.go`, `routes/w20_project_update_delete_race_test.go`, `routes/w21_tasktodo_update_rollback_race_test.go` | Regression tests of bugs found by probing this repository's route table (board rows W11 to W21). |

## Tests changed here to follow the hub

Four tests that only this repository has had to change because the hub's behaviour moved on:

| Test | Change |
|---|---|
| `routes/case_vote_test.go` | A vote counts for the caller (hub 1893a960, MUN133), so neema's vote is cast as neema and a body `member_id` naming another member is ignored. |
| `codesequence_test.go`, `TestConcurrentDistinctTagsEachGetTheirOwnCode` | Creates its issues one after another and races only the tag-minting updates. Numbering a new issue (hub c6b0bafa) reads the project's codes before it writes the counter, which on a shared-cache SQLite database with several connections deadlocks (`database table is locked`). Postgres is unaffected. Concurrent issue creation on that kind of SQLite database is not covered, and fixing it means changing the hub copy and this repository together, which is for the owner to decide. |
| `workflow_run_rollback_test.go`, `workflow_run_version_test.go`, `workflow_run_cancel_test.go`, `postgres_integration_test.go` | The helper `contains` is renamed `listHas` because the hub's `backlog_rules_test.go` declares `contains` for strings in the same package. |
| `generated_test.go`, `sourced_test.go`, `testrun_test.go` | Their request helpers take new names (`generatedTasksRequest`, `sourcedTasksRequest`) because the hub's `backlog_access_sourced_test.go` declares `sourcedRequest` and `generateRequest`. |

## Verification run

On a `git archive` export of this repository beside an export of `mwanachama-backend-shared`, with a temporary `replace` in the scratch copy only (this repository's `go.mod` is unchanged): `go build ./...`, `go vet ./...` and `go test -race -p 2 -timeout 40m ./...`, one command at a time. The results are recorded in the log of hub issue W72.
