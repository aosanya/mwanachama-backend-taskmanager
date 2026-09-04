// Package mwanachamataskmanager — pre-delivered schema definition.
//
// This file exposes [DefaultWorkSchema], which returns the fixed
// [schema.Schema] for mwanachama-backend-taskmanager. Wiring code in
// mwanachama-backend-api-gateway seeds this schema at startup via
// SchemaManager.SetSchema.
//
// The schema declares eight TypeDefinitions:
//   - Task               — a unit of work assigned to an AI Agent (mutable)
//   - TaskTodo           — a decomposed sub-task produced by a decomposition run; carries todo_type and max_runs for per-type run-count enforcement (mutable)
//   - Project            — optional container that groups related Tasks via `member_of` edges
//   - Agent              — Work-domain projection of an AI agent; vertex for `assigned_to` edges
//   - Tag                — free-form label attached to Tasks via `has_tag` edges
//   - ImportProjectJob   — tracks async project-import operations
//   - Deliverable        — specification of what a Task or TaskTodo must produce
//   - AcceptanceCriteria — verifiable condition linked to a Task or TaskTodo; carries a reviewer-written result
//
// Graph topology:
//
//	Task     ──assigned_to────────────► Agent
//	Task     ──member_of──────────────► Project
//	Task     ──blocks──────────────────► Task
//	Task     ──subtask_of──────────────► Task
//	Task     ──depends_on──────────────► Task
//	Task     ──has_tag──────────────────► Tag
//	Task     ──has_todo─────────────────► TaskTodo
//	Task     ──has_deliverable──────────► Deliverable
//	Task     ──has_acceptance_criteria──► AcceptanceCriteria
//	TaskTodo ──has_deliverable──────────► Deliverable
//	TaskTodo ──has_acceptance_criteria──► AcceptanceCriteria
//
// Storage: every entity lives in mwanachama-backend-shared's single Postgres
// `entities` table, keyed by TypeID; TypeDefinition.StorageCollection below
// is carried over from the ArangoDB original purely as a label (see
// schema.TypeDefinition's doc) and has no functional effect here. All edges
// live in the single `relationships` table.
//
// Ported from github.com/aosanya/CodeValdWork's schema.go, retargeted onto
// mwanachama-backend-shared's schema.* types. Dropped along the way:
// PathSegment/EntityIDParam/PublishEvents (route/topic-generation fields the
// new schema type doesn't carry — mwanachama-backend-api-gateway registers routes
// and events by hand) and the eventreceiver.ReceivedEventTypeDefinition("work")
// entry appended by the original (CodeValdCortex-specific event-tracking
// plumbing with no equivalent here). Reset from the original's Version: 4 /
// Tag: "v4" (four schema iterations accumulated over CodeValdWork's history)
// to Version: 1 / Tag: "v1" — this is schema history starting fresh in a new
// project, with the original's cumulative v1-v4 properties already folded
// into this single definition.
package mwanachamataskmanager

import "github.com/aosanya/mwanachama-backend-shared/schema"

// DefaultWorkSchema returns the pre-delivered [schema.Schema] seeded by
// mwanachama-backend-api-gateway on startup via SchemaManager.SetSchema. The
// operation is idempotent — calling it multiple times with the same schema
// ID is safe.
func DefaultWorkSchema() schema.Schema {
	return schema.Schema{
		ID:      "work-schema-v1",
		Version: 1,
		Tag:     "v1",
		Types: []schema.TypeDefinition{
			{
				Name:              "Task",
				DisplayName:       "Task",
				StorageCollection: "work_tasks",
				Properties: []schema.PropertyDefinition{
					// title is the short human-readable label (e.g. "Farm Dashboard").
					{Name: "title", Type: schema.PropertyTypeString},
					// description provides additional context for the assigned agent.
					{Name: "description", Type: schema.PropertyTypeString},
					// status is the current lifecycle state — see [TaskStatus].
					// Well-known values: "pending", "in_progress", "completed", "failed", "cancelled".
					{Name: "status", Type: schema.PropertyTypeString},
					// priority indicates relative urgency — see [TaskPriority].
					// Well-known values: "low", "medium", "high", "critical".
					{Name: "priority", Type: schema.PropertyTypeString},
					// due_at is the RFC 3339 deadline; empty when no deadline is set.
					{Name: "due_at", Type: schema.PropertyTypeString},
					// tags are free-form labels associated with the task.
					{Name: "tags", Type: schema.PropertyTypeArray, ElementType: schema.PropertyTypeString},
					// estimated_hours is the planned effort to complete the task, in hours.
					{Name: "estimated_hours", Type: schema.PropertyTypeNumber},
					// context is the AI agent's working memory blob.
					{Name: "context", Type: schema.PropertyTypeString},
					// completed_at is set when status reaches a terminal state (RFC 3339).
					{Name: "completed_at", Type: schema.PropertyTypeString},
					// task_name is the project-scoped auto-generated name (e.g. "MVP-001").
					{Name: "task_name", Type: schema.PropertyTypeString},
					// project_name is the URL-safe slug of the project this task belongs to.
					{Name: "project_name", Type: schema.PropertyTypeString},
					// separate_branch indicates whether this task should be worked on in its own git branch.
					{Name: "separate_branch", Type: schema.PropertyTypeBoolean},
					// branch_name is the git branch to create/use for this task (e.g. "feature/SF-001_scaffolding").
					{Name: "branch_name", Type: schema.PropertyTypeString},
					// workflow_run_id denormalises the WorkflowRun anchor onto the
					// Task row so queries can filter by run-id without traversing
					// the started_task edge. Empty for tasks not produced under a run.
					{Name: "workflow_run_id", Type: schema.PropertyTypeString},
					// recovery_runs_used counts the automatic retries consumed so far
					// for this task. Compared to max_recovery_runs (default: 3) to
					// decide when to escalate to AI classification.
					{Name: "recovery_runs_used", Type: schema.PropertyTypeInteger},
					// blocker_note is the human-readable reason stored when the
					// mark-blocked direction option is chosen.
					{Name: "blocker_note", Type: schema.PropertyTypeString},
					// direction_history is a JSON-encoded []string of past selected_option
					// values submitted for this task via work.task.direction events.
					{Name: "direction_history", Type: schema.PropertyTypeString},
					// parent_task_id is the ID of the Task that was split to produce
					// this child. Empty for root tasks. Denormalises the subtask_of
					// edge so child lookups do not require a graph traversal.
					{Name: "parent_task_id", Type: schema.PropertyTypeString},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
				},
				Relationships: []schema.RelationshipDefinition{
					{
						Name:    RelLabelAssignedTo,
						Label:   "Assigned to",
						ToType:  "Agent",
						ToMany:  false,
						Inverse: "assigned_tasks",
						Properties: []schema.PropertyDefinition{
							{Name: "assigned_at", Type: schema.PropertyTypeString},
							{Name: "assigned_by", Type: schema.PropertyTypeString},
						},
					},
					{
						Name:    RelLabelBlocks,
						Label:   "Blocks",
						ToType:  "Task",
						ToMany:  true,
						Inverse: "blocked_by",
						Properties: []schema.PropertyDefinition{
							{Name: "created_at", Type: schema.PropertyTypeString},
							{Name: "reason", Type: schema.PropertyTypeString},
						},
					},
					{
						Name:    RelLabelSubtaskOf,
						Label:   "Subtask of",
						ToType:  "Task",
						ToMany:  false,
						Inverse: "has_subtask",
						Properties: []schema.PropertyDefinition{
							{Name: "created_at", Type: schema.PropertyTypeString},
						},
					},
					{
						Name:    RelLabelDependsOn,
						Label:   "Depends on",
						ToType:  "Task",
						ToMany:  true,
						Inverse: "depended_on_by",
						Properties: []schema.PropertyDefinition{
							{Name: "created_at", Type: schema.PropertyTypeString},
							{Name: "reason", Type: schema.PropertyTypeString},
						},
					},
					{
						Name:    RelLabelMemberOf,
						Label:   "Member of",
						ToType:  "Project",
						ToMany:  true,
						Inverse: "has_task",
						Properties: []schema.PropertyDefinition{
							{Name: "added_at", Type: schema.PropertyTypeString},
						},
					},
					{
						Name:    RelLabelHasTag,
						Label:   "Has tag",
						ToType:  "Tag",
						ToMany:  true,
						Inverse: "tagged_tasks",
						Properties: []schema.PropertyDefinition{
							{Name: "tagged_at", Type: schema.PropertyTypeString},
						},
					},
					{
						Name:    "blocked_by",
						Label:   "Blocked by",
						ToType:  "Task",
						ToMany:  true,
						Inverse: RelLabelBlocks,
					},
					{
						Name:    "has_subtask",
						Label:   "Subtasks",
						ToType:  "Task",
						ToMany:  true,
						Inverse: RelLabelSubtaskOf,
					},
					{
						Name:    "depended_on_by",
						Label:   "Depended on by",
						ToType:  "Task",
						ToMany:  true,
						Inverse: RelLabelDependsOn,
					},
					{
						Name:    RelLabelHasTodo,
						Label:   "Todos",
						ToType:  "TaskTodo",
						ToMany:  true,
						Inverse: "todo_of",
					},
					{
						Name:    RelLabelPartOfRun,
						Label:   "Part of run",
						ToType:  "WorkflowRun",
						ToMany:  false,
						Inverse: RelLabelStartedTask,
					},
					{
						Name:    RelLabelHasDeliverable,
						Label:   "Has deliverable",
						ToType:  "Deliverable",
						ToMany:  true,
						Inverse: "deliverable_of",
					},
					{
						Name:    RelLabelHasAcceptanceCriteria,
						Label:   "Has acceptance criteria",
						ToType:  "AcceptanceCriteria",
						ToMany:  true,
						Inverse: "criteria_of",
					},
				},
			},
			{
				Name:              "TaskTodo",
				DisplayName:       "Task Todo",
				StorageCollection: "work_task_todos",
				Properties: []schema.PropertyDefinition{
					// title is the short label for this sub-task.
					{Name: "title", Type: schema.PropertyTypeString, Required: true},
					// description explains what this sub-task accomplishes.
					{Name: "description", Type: schema.PropertyTypeString},
					// instructions is the fully self-contained agent prompt for executing this todo.
					{Name: "instructions", Type: schema.PropertyTypeString, Required: true},
					// ordinality is the 1-based position of this todo within the decomposition.
					{Name: "ordinality", Type: schema.PropertyTypeInteger, Required: true},
					// can_run_parallel is true when this todo has no predecessor dependency.
					{Name: "can_run_parallel", Type: schema.PropertyTypeBoolean},
					// depends_on is a JSON-encoded []int of ordinality values that must complete first.
					{Name: "depends_on", Type: schema.PropertyTypeArray, ElementType: schema.PropertyTypeInteger},
					// status tracks the todo lifecycle: pending → dispatched → completed | failed.
					{Name: "status", Type: schema.PropertyTypeString},
					// parent_task_id is the Task ID from which this todo was decomposed.
					{Name: "parent_task_id", Type: schema.PropertyTypeString, Required: true},
					// decomp_run_id is the AgentRun ID that produced this todo.
					{Name: "decomp_run_id", Type: schema.PropertyTypeString},
					// agent_id is the agent assigned to execute this todo.
					{Name: "agent_id", Type: schema.PropertyTypeString},
					// precalls is a JSON-encoded []PrecallSpec: pre-execution fetch specs whose
					// results are injected into the LLM context before the agent runs. Each spec
					// targets a specific service (e.g. "git") and operation (e.g. "blob_search")
					// with typed parameters.
					{Name: "precalls", Type: schema.PropertyTypeString},
					// todo_type is the semantic type of this todo (e.g. "compile-fix"). Run count
					// per (parent_task_id, todo_type) is tracked and max_runs is enforced at
					// creation time — rejecting the injection when the limit is reached.
					{Name: "todo_type", Type: schema.PropertyTypeString},
					// max_runs is the maximum number of todos of this todo_type that may be created
					// for the parent task. Enforced at creation time. Zero means no limit.
					{Name: "max_runs", Type: schema.PropertyTypeInteger},
					// workflow_run_id denormalises the WorkflowRun anchor onto the
					// TaskTodo row. Inherited from the parent Task at creation
					// time so the todo carries the run-id its parent belongs to.
					{Name: "workflow_run_id", Type: schema.PropertyTypeString},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
				},
				Relationships: []schema.RelationshipDefinition{
					{
						Name:     "todo_of",
						Label:    "Parent Task",
						ToType:   "Task",
						ToMany:   false,
						Required: true,
						Inverse:  RelLabelHasTodo,
					},
					{
						Name:    RelLabelTodoAssignedTo,
						Label:   "Assigned to",
						ToType:  "Agent",
						ToMany:  false,
						Inverse: "todo_assigned_tasks",
					},
					{
						Name:    RelLabelPartOfRun,
						Label:   "Part of run",
						ToType:  "WorkflowRun",
						ToMany:  false,
						Inverse: RelLabelStartedTodo,
					},
					{
						Name:    RelLabelHasDeliverable,
						Label:   "Has deliverable",
						ToType:  "Deliverable",
						ToMany:  true,
						Inverse: "deliverable_of",
					},
					{
						Name:    RelLabelHasAcceptanceCriteria,
						Label:   "Has acceptance criteria",
						ToType:  "AcceptanceCriteria",
						ToMany:  true,
						Inverse: "criteria_of",
					},
				},
			},
			{
				Name:              "Project",
				DisplayName:       "Project",
				StorageCollection: "work_projects",
				Properties: []schema.PropertyDefinition{
					// name is the short human-readable label. Required.
					{Name: "name", Type: schema.PropertyTypeString, Required: true},
					// project_name is the URL-safe slug (lowercase, spaces→underscores).
					{Name: "project_name", Type: schema.PropertyTypeString},
					// description provides additional context for the project.
					{Name: "description", Type: schema.PropertyTypeString},
					// repo_name is the mwanachama-backend-git repository name associated with this project.
					{Name: "repo_name", Type: schema.PropertyTypeString},
					// github_repo is the canonical GitHub repository, e.g. "owner/name".
					{Name: "github_repo", Type: schema.PropertyTypeString},
					// task_prefix is prepended to the counter when auto-generating task names.
					{Name: "task_prefix", Type: schema.PropertyTypeString},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
				},
				Relationships: []schema.RelationshipDefinition{
					{
						Name:    "has_task",
						Label:   "Tasks",
						ToType:  "Task",
						ToMany:  true,
						Inverse: RelLabelMemberOf,
					},
				},
			},
			{
				Name:              "Agent",
				DisplayName:       "Agent",
				StorageCollection: "work_agents",
				// UniqueKey on agent_id makes the external identifier the natural key for
				// UpsertEntity — UpsertAgent relies on this for find-or-create.
				UniqueKey: []string{"agent_id"},
				Properties: []schema.PropertyDefinition{
					// agent_id is the external agent identifier. Globally unique.
					{Name: "agent_id", Type: schema.PropertyTypeString, Required: true},
					// display_name is a human-readable label for the agent.
					{Name: "display_name", Type: schema.PropertyTypeString},
					// capability is the agent's primary capability (e.g. "code", "research").
					{Name: "capability", Type: schema.PropertyTypeString},
					// role_name is the role this agent fulfils (e.g. "domain-expert").
					{Name: "role_name", Type: schema.PropertyTypeString},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
				},
				Relationships: []schema.RelationshipDefinition{
					{
						Name:    "assigned_tasks",
						Label:   "Assigned Tasks",
						ToType:  "Task",
						ToMany:  true,
						Inverse: RelLabelAssignedTo,
					},
					{
						Name:    "todo_assigned_tasks",
						Label:   "Assigned Todo Tasks",
						ToType:  "TaskTodo",
						ToMany:  true,
						Inverse: RelLabelTodoAssignedTo,
					},
				},
			},
			{
				Name:              "Tag",
				DisplayName:       "Tag",
				StorageCollection: "work_tags",
				UniqueKey:         []string{"name"},
				Properties: []schema.PropertyDefinition{
					// name is the unique label text (e.g. "setup", "auth").
					{Name: "name", Type: schema.PropertyTypeString, Required: true},
					// color is an optional hex/CSS color for UI rendering.
					{Name: "color", Type: schema.PropertyTypeString},
					// description provides additional context for the tag.
					{Name: "description", Type: schema.PropertyTypeString},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
				},
				Relationships: []schema.RelationshipDefinition{
					{
						Name:    "tagged_tasks",
						Label:   "Tagged Tasks",
						ToType:  "Task",
						ToMany:  true,
						Inverse: RelLabelHasTag,
					},
				},
			},
			{
				Name:              "WorkflowRun",
				DisplayName:       "Workflow Run",
				StorageCollection: "work_workflow_runs",
				// UniqueKey on name guarantees the name maps to at most one
				// run vertex, so callers can correlate by a caller-supplied
				// or server-generated label.
				UniqueKey: []string{"name"},
				Properties: []schema.PropertyDefinition{
					// name is a caller-supplied or server-generated label,
					// globally unique. Used as the correlation handle by
					// test scripts and the headline column in the UI list.
					{Name: "name", Type: schema.PropertyTypeString},
					// status is the run lifecycle state: pending, in_progress,
					// completed, failed, rolled_back, ...
					{Name: "status", Type: schema.PropertyTypeString},
					// trigger_event names the event that started the run
					// (e.g. "next.requested").
					{Name: "trigger_event", Type: schema.PropertyTypeString},
					// initiator is an opaque caller identifier (operator email,
					// service name, etc.). May be empty.
					{Name: "initiator", Type: schema.PropertyTypeString},
					// notes is free-form human-readable context.
					{Name: "notes", Type: schema.PropertyTypeString},
					// agent_run_ids, function_job_ids, branch_names are stored
					// as JSON-encoded string arrays — the cross-service references
					// the closure read surfaces.
					{Name: "agent_run_ids", Type: schema.PropertyTypeArray, ElementType: schema.PropertyTypeString},
					{Name: "function_job_ids", Type: schema.PropertyTypeArray, ElementType: schema.PropertyTypeString},
					{Name: "branch_names", Type: schema.PropertyTypeArray, ElementType: schema.PropertyTypeString},
					// terminal_event is a colon-delimited condition that auto-completes the run.
					// Format: "topic:field=value:field=value"
					// Example: "functions.job.completed:function_name=merge-flutter-branch:status=ok"
					// Empty = run never auto-completes (operator must close it manually).
					{Name: "terminal_event", Type: schema.PropertyTypeString},
					{Name: "started_at", Type: schema.PropertyTypeString},
					{Name: "completed_at", Type: schema.PropertyTypeString},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
					// parent_workflow_run_id references the WorkflowRun whose
					// failure spawned this child (recovery) run. Empty for
					// top-level runs.
					{Name: "parent_workflow_run_id", Type: schema.PropertyTypeString},
					// root_workflow_run_id is denormalised — the topmost ancestor
					// of the recovery chain. For a top-level run this equals the
					// run's own ID (or is empty; consumers default it). Used for
					// O(1) chain aggregation.
					{Name: "root_workflow_run_id", Type: schema.PropertyTypeString},
					// failure_pipeline_budget is the maximum number of recovery
					// pipeline activations allowed under this run's lineage.
					// Resolved at start-pipeline time (payload override > global default)
					// and frozen for the run's lifetime. Lives only on the root run.
					{Name: "failure_pipeline_budget", Type: schema.PropertyTypeInteger},
					// failure_pipelines_used counts recovery activations charged
					// to this root run so far. Incremented atomically by
					// IncrementFailureBudget. Lives only on the root run.
					{Name: "failure_pipelines_used", Type: schema.PropertyTypeInteger},
					// counted_child_run_ids is the set of child run IDs already
					// charged to failure_pipelines_used. Used for idempotency on
					// dispatch retry — a repeated IncrementFailureBudget with the
					// same child_run_id returns the current counter without
					// double-incrementing. Stored JSON-encoded.
					{Name: "counted_child_run_ids", Type: schema.PropertyTypeArray, ElementType: schema.PropertyTypeString},
					// cancelled_by, cancel_reason, cancelling_until carry the
					// cancellation envelope onto the run row so it survives
					// process restart and is visible to closure reads / list views.
					{Name: "cancelled_by", Type: schema.PropertyTypeString},
					{Name: "cancel_reason", Type: schema.PropertyTypeString},
					{Name: "cancelling_until", Type: schema.PropertyTypeString},
					// last_event_at is the RFC3339 UTC timestamp of the most recent
					// event observed that carried this run's workflow_run_id.
					// Updated best-effort on every matching event publish.
					// Initialised to created_at. Used by the watchdog sweeper to
					// detect stale runs.
					{Name: "last_event_at", Type: schema.PropertyTypeString},
					// timeout_published is true once work.run.timeout has been
					// emitted for this run. Prevents duplicate timeout events on
					// process restart.
					{Name: "timeout_published", Type: schema.PropertyTypeBoolean},
					// paused_at is set by a future operator-pause endpoint. A
					// non-null value causes the watchdog to skip this run entirely.
					{Name: "paused_at", Type: schema.PropertyTypeString},
					// current_step_id is the plan code of the step currently
					// executing. Set when the step's trigger event is dispatched;
					// cleared when the step's success or failure event is observed.
					// Used by the per-step timeout sweep.
					{Name: "current_step_id", Type: schema.PropertyTypeString},
					// current_step_started_at is the RFC3339 UTC timestamp when the
					// current step was dispatched. Used with the step's step_timeout
					// to detect stalled steps.
					{Name: "current_step_started_at", Type: schema.PropertyTypeString},
				},
				Relationships: []schema.RelationshipDefinition{
					{
						Name:    RelLabelStartedTask,
						Label:   "Started task",
						ToType:  "Task",
						ToMany:  true,
						Inverse: RelLabelPartOfRun,
						Properties: []schema.PropertyDefinition{
							{Name: "created_at", Type: schema.PropertyTypeString},
						},
					},
					{
						Name:    RelLabelStartedTodo,
						Label:   "Started todo",
						ToType:  "TaskTodo",
						ToMany:  true,
						Inverse: RelLabelPartOfRun,
						Properties: []schema.PropertyDefinition{
							{Name: "created_at", Type: schema.PropertyTypeString},
						},
					},
				},
			},
			{
				Name:              "ImportProjectJob",
				DisplayName:       "Import Project Job",
				StorageCollection: "work_import_jobs",
				Properties: []schema.PropertyDefinition{
					// status tracks the async lifecycle: pending, running, completed, failed, cancelled.
					{Name: "status", Type: schema.PropertyTypeString},
					// error_message is populated when status is "failed".
					{Name: "error_message", Type: schema.PropertyTypeString},
					// tasks_created is the number of Task vertices written on completion.
					{Name: "tasks_created", Type: schema.PropertyTypeNumber},
					// deps_created is the number of depends_on edges written on completion.
					{Name: "deps_created", Type: schema.PropertyTypeNumber},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
				},
			},
			{
				Name:              "Deliverable",
				DisplayName:       "Deliverable",
				StorageCollection: "work_deliverables",
				Properties: []schema.PropertyDefinition{
					// title is the short human-readable label.
					{Name: "title", Type: schema.PropertyTypeString},
					// description is the fuller spec of what must be produced.
					{Name: "description", Type: schema.PropertyTypeString},
					// deliverable_type classifies the output (e.g. "code", "document", "artifact", "test_output").
					{Name: "deliverable_type", Type: schema.PropertyTypeString},
					// parent_id is the denormalised owner ID (Task or TaskTodo).
					{Name: "parent_id", Type: schema.PropertyTypeString},
					// ordinality is the 1-based position within the owning entity's deliverables.
					{Name: "ordinality", Type: schema.PropertyTypeInteger},
					// workflow_run_id is inherited from the parent at creation time.
					{Name: "workflow_run_id", Type: schema.PropertyTypeString},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
				},
				Relationships: []schema.RelationshipDefinition{
					{
						Name:    "deliverable_of",
						Label:   "Deliverable of",
						ToType:  "Task",
						ToMany:  false,
						Inverse: RelLabelHasDeliverable,
					},
				},
			},
			{
				Name:              "AcceptanceCriteria",
				DisplayName:       "Acceptance Criteria",
				StorageCollection: "work_acceptance_criteria",
				Properties: []schema.PropertyDefinition{
					// title is the short label (e.g. "All unit tests pass with race detector").
					{Name: "title", Type: schema.PropertyTypeString},
					// description is the full verifiable condition.
					{Name: "description", Type: schema.PropertyTypeString},
					// parent_id is the denormalised owner ID (Task or TaskTodo).
					{Name: "parent_id", Type: schema.PropertyTypeString},
					// ordinality is the 1-based position within the owning entity's criteria.
					{Name: "ordinality", Type: schema.PropertyTypeInteger},
					// workflow_run_id is inherited from the parent at creation time.
					{Name: "workflow_run_id", Type: schema.PropertyTypeString},
					// result is the runtime outcome written by the reviewer: "passed", "failed", "skipped", "blocked".
					// Empty until the reviewer runs.
					{Name: "result", Type: schema.PropertyTypeString},
					// result_notes is the free-form explanation written by the reviewer alongside result.
					{Name: "result_notes", Type: schema.PropertyTypeString},
					{Name: "created_at", Type: schema.PropertyTypeString},
					{Name: "updated_at", Type: schema.PropertyTypeString},
				},
				Relationships: []schema.RelationshipDefinition{
					{
						Name:    "criteria_of",
						Label:   "Criteria of",
						ToType:  "Task",
						ToMany:  false,
						Inverse: RelLabelHasAcceptanceCriteria,
					},
				},
			},
		},
	}
}
