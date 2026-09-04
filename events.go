// Package mwanachamataskmanager — event topics and payloads.
//
// Every Topic* constant here is a plain string literal — the original
// CodeValdWork built these as `eventbus.DomainWork + "task.created"`, but
// DomainWork had already been blanked to "" platform-wide (intent-keyed
// topic names, no per-service domain prefix) before this port started, so
// the concatenation added nothing worth keeping.
//
// Dropped from the original: AllTopics() and ConsumedTopics() — both existed
// only to feed internal/registrar's Cross topic registration and the
// TaskEventDispatcher's inbound subscriptions, neither of which is ported
// here (see CLAUDE.md). The schema-derived half of AllTopics() also had no
// replacement available: mwanachama-backend-shared's schema package dropped
// TopicsFromSchema/PathSegment/PublishEvents entirely (no schema-derived
// route or topic generation — see its S2 notes).
//
// The `publish` helper that actually calls [Publisher.Publish] is not part
// of this file in the original either — it lives alongside CreateTask et al.
// (task_impl_task.go) and lands with W4.
package mwanachamataskmanager

// Event topic constants — the closed set this package publishes.
const (
	// TopicTaskCreated fires after a Task entity is created.
	// Payload: [TaskCreatedPayload].
	TopicTaskCreated = "task.created"

	// TopicTaskUpdated fires after a non-status mutable field changes.
	// Payload: [TaskUpdatedPayload].
	TopicTaskUpdated = "task.updated"

	// TopicTaskStatusChanged fires on every successful status transition.
	// Payload: [TaskStatusChangedPayload].
	TopicTaskStatusChanged = "task.status.changed"

	// TopicTaskCompleted fires when a transition reaches a terminal status
	// (completed, failed, cancelled). Published in addition to
	// [TopicTaskStatusChanged]. Payload: [TaskCompletedPayload].
	TopicTaskCompleted = "task.completed"

	// TopicTaskFailed fires when an agent run fails to satisfy the required
	// output contract (e.g. no actions block emitted). Published in addition
	// to [TopicTaskCompleted] when the failure is agent-driven.
	// Payload: [TaskFailedPayload].
	TopicTaskFailed = "task.failed"

	// TopicTaskAssigned fires when an `assigned_to` edge is created or
	// replaced. Payload: [TaskAssignedPayload].
	TopicTaskAssigned = "task.assigned"

	// TopicRelationshipCreated fires when any whitelisted graph edge is
	// created. Payload: [RelationshipCreatedPayload].
	TopicRelationshipCreated = "relationship.created"

	// TopicTodoDispatched fires when a [TaskTodo] entity is created — once
	// per todo item produced by a todo.created decomposition payload.
	// Downstream agents subscribe to this topic via work plans and execute
	// each todo. Payload: [TodoDispatchedPayload].
	TopicTodoDispatched = "todo.dispatched"

	// TopicTodoCompleted fires when a TaskTodo reaches a terminal status
	// (completed or failed). Carries todo_type and max_runs so downstream
	// consumers (e.g. compile-on-todo-completed) can act without fetching
	// the entity separately.
	// Payload: [TodoCompletedPayload].
	TopicTodoCompleted = "todo.completed"

	// TopicTaskUpdate is consumed by this package to patch mutable task
	// fields. Published by an upstream agent step when the LLM emits a
	// work.task.update action (e.g. after choosing a branch name).
	// Currently only branch_name is patched.
	// Payload: [TaskUpdatePayload].
	TopicTaskUpdate = "task.update"

	// TopicTaskRolledBack fires once per Task deleted by [TaskManager.DeleteWorkflowRunArtifacts].
	// Payload: [TaskRolledBackPayload].
	TopicTaskRolledBack = "task.rolled_back"

	// TopicTaskCancelled fires once per non-terminal Task whose status was
	// flipped to cancelled by the run-cancel cascade. Subscribers drop
	// in-flight work for the task. Payload: [TaskCancelledPayload].
	TopicTaskCancelled = "task.cancelled"

	// TopicPipelineStarted is published by an upstream orchestrator
	// immediately after a WorkflowRun is minted. This package subscribes so
	// the run-status handler can flip the run from pending → in_progress
	// without waiting for the first work.task.assigned event.
	TopicPipelineStarted = "pipeline.started"

	// TopicTaskNeedsDirection is published when a task has exhausted its
	// retry budget and requires direction to resume. The payload carries a
	// DirectionForm JSON object renderable on any platform.
	// Payload: [TaskNeedsDirectionPayload].
	TopicTaskNeedsDirection = "task.needs-direction"

	// TopicTaskDirection is consumed from the bus — emitted by an AI
	// failure-direction handler or the frontend after a human resolves the
	// direction form. Routes to the direction handler which resumes the
	// task according to selected_option.
	// Payload: [TaskDirectionPayload].
	TopicTaskDirection = "task.direction"

	// TopicTaskClassifyFailure is published after a task exhausts its
	// automatic retry budget. An AI classifier subscribes and responds with
	// [TopicTaskFailureClassified]. Payload: [TaskClassifyFailurePayload].
	TopicTaskClassifyFailure = "task.classify-failure"

	// TopicTaskFailureClassified is consumed here — emitted by an AI
	// classifier in response to [TopicTaskClassifyFailure]. Carries a
	// failure_type of "transient" or "requires-human" so this package can
	// decide whether to grant one extra retry or escalate to human direction.
	// Payload: [TaskFailureClassifiedPayload].
	TopicTaskFailureClassified = "task.failure-classified"

	// TopicRunPaused fires when a WorkflowRun transitions to paused status
	// because at least one task is awaiting direction.
	// Payload: [RunPausedPayload].
	TopicRunPaused = "run.paused"

	// TopicRunResumed fires when a paused WorkflowRun transitions back to
	// in_progress after all awaiting-direction tasks have been resolved.
	// Payload: [RunResumedPayload].
	TopicRunResumed = "run.resumed"

	// TopicTaskPlanSplit is emitted by an AI planner agent when it decides
	// to break a task into child Task entities instead of decomposing it
	// into todos. This package consumes it to create the child tasks, write
	// subtask_of edges, and transition the parent to TaskStatusSplit.
	// Payload: [TaskPlanSplitPayload].
	TopicTaskPlanSplit = "task.request-split"

	// TopicReviewPassed fires when the reviewer evaluates all
	// AcceptanceCriteria for a completed task and every criterion has
	// result == "passed". Payload: [ReviewOutcomePayload].
	TopicReviewPassed = "review.passed"

	// TopicReviewFailed fires when the reviewer evaluates AcceptanceCriteria
	// for a completed task and at least one criterion has result !=
	// "passed". Payload: [ReviewOutcomePayload].
	TopicReviewFailed = "review.failed"
)

// TaskCreatedPayload is the Publish payload for [TopicTaskCreated].
type TaskCreatedPayload struct {
	TaskID   string
	Priority TaskPriority
	// WorkflowRunID is the WorkflowRun anchor this task belongs to, or empty
	// when the task was created outside an orchestrated run (chain-through
	// rule).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskUpdatedPayload is the Publish payload for [TopicTaskUpdated].
type TaskUpdatedPayload struct {
	TaskID        string
	ChangedFields []string
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskStatusChangedPayload is the Publish payload for [TopicTaskStatusChanged].
type TaskStatusChangedPayload struct {
	TaskID string
	From   TaskStatus
	To     TaskStatus
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskCompletedPayload is the Publish payload for [TopicTaskCompleted].
type TaskCompletedPayload struct {
	TaskID         string
	TerminalStatus TaskStatus
	// CompletedAt is the RFC 3339 timestamp when the terminal status was set.
	CompletedAt string
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskFailedBy identifies the agent and work plan responsible for a task failure.
type TaskFailedBy struct {
	AgentID      string
	WorkPlanID   string
	WorkPlanCode string
}

// TaskFailedPayload is the Publish payload for [TopicTaskFailed].
// Consumers needing the raw LLM output should fetch the AgentRun using RunID.
type TaskFailedPayload struct {
	TaskID   string
	RunID    string
	Reason   string
	FailedBy TaskFailedBy
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskRolledBackPayload is the Publish payload for [TopicTaskRolledBack].
// Emitted once per Task deleted by [TaskManager.DeleteWorkflowRunArtifacts].
type TaskRolledBackPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id"`
}

// TaskAssignedPayload is the Publish payload for [TopicTaskAssigned].
type TaskAssignedPayload struct {
	TaskID      string
	AgentID     string
	RoleName    string
	TaskCode    string // project-scoped code, e.g. "UTIL-001" — empty for tasks not in a project
	Title       string
	Description string
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// RelationshipCreatedPayload is the Publish payload for [TopicRelationshipCreated].
type RelationshipCreatedPayload struct {
	FromID string
	ToID   string
	Label  string
}

// TodoDispatchedPayload is the Publish payload for [TopicTodoDispatched].
// Published once per TaskTodo entity created from a todo.created decomposition.
//
// Field identity contract (important for consumers):
//   - TodoID       is the identity of the todo itself. The AI service sets
//     AgentRun.TaskID to this value so that task.completed/failed events
//     route back to the todo (via updateTodoStatus), not to the parent task.
//   - TaskID       equals ParentTaskID and exists only for context-hydration
//     steps that need the parent task ID under the key "TaskID" to fetch task
//     context. It MUST NOT be used to set AgentRun.TaskID.
//   - ParentTaskID is the canonical parent task identifier.
type TodoDispatchedPayload struct {
	TodoID         string
	TaskID         string // equals ParentTaskID; for context hydration only — do not use as AgentRun.TaskID
	ParentTaskID   string
	DecompRunID    string
	AgentID        string
	Title          string
	Instructions   string
	Ordinality     int
	CanRunParallel bool
	DependsOn      []int
	Precalls       string // JSON-encoded []PrecallSpec stored on the TaskTodo
	TodoType       string // semantic type label (e.g. "compile-fix"); used for per-type run-count enforcement
	MaxRuns        int    // maximum spawns of this todo type within the parent task; 0 means no limit
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TodoCompletedPayload is the Publish payload for [TopicTodoCompleted].
// Published when a TaskTodo reaches a terminal status (completed or failed).
type TodoCompletedPayload struct {
	TodoID       string `json:"todo_id"`
	ParentTaskID string `json:"task_id"` // keyed "task_id" so downstream dispatchers can extract it
	Title        string `json:"title"`
	Status       string `json:"status"`              // "completed" or "failed"
	TodoType     string `json:"todo_type"`           // forwarded from the TaskTodo entity; empty when no type was set
	MaxRuns      int    `json:"max_runs,omitempty"`  // forwarded from the TaskTodo entity; 0 means no limit
	RunCount     int    `json:"run_count,omitempty"` // current count of todos of this TodoType for the parent task at the time of completion
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskPlanSplitChildSpec describes one child Task to be created by the
// split-handler on receipt of [TopicTaskPlanSplit].
type TaskPlanSplitChildSpec struct {
	// TempID is a planner-assigned string used to resolve depends_on references
	// within the same payload. Not stored on the created Task entity.
	TempID      string `json:"temp_id"`
	TaskName    string `json:"task_name,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	RoleName    string `json:"role_name,omitempty"`
	// DependsOn lists TempID values of sibling children that must complete
	// before this child is dispatched.
	DependsOn []string `json:"depends_on,omitempty"`
}

// TaskPlanSplitPayload is the Publish payload for [TopicTaskPlanSplit].
// Emitted by an AI planner agent when it decides to split a task.
type TaskPlanSplitPayload struct {
	TaskID        string                   `json:"task_id"`
	WorkflowRunID string                   `json:"workflow_run_id,omitempty"`
	Children      []TaskPlanSplitChildSpec `json:"children"`
}

// TaskUpdatePayload is the Publish payload for [TopicTaskUpdate].
// Published by an upstream agent step when the LLM emits a work.task.update action.
type TaskUpdatePayload struct {
	// TaskID is the Task entity ID to patch.
	TaskID string `json:"task_id"`
	// BranchName is the git branch the AI agent created for this task.
	// Written back so downstream context-hydration steps can use it for
	// file hydration.
	BranchName string `json:"branch_name,omitempty"`
}

// WorkflowRun status event topic constants.
const (
	// TopicRunInProgress fires when a WorkflowRun first transitions to in_progress.
	TopicRunInProgress = "run.in_progress"
	// TopicRunCompleted fires when a WorkflowRun reaches the completed terminal state.
	TopicRunCompleted = "run.completed"
	// TopicRunFailed fires when a WorkflowRun reaches the failed terminal state.
	TopicRunFailed = "run.failed"
	// TopicRunRolledBack fires when a WorkflowRun reaches the rolled_back terminal state.
	TopicRunRolledBack = "run.rolled_back"
	// TopicRunRollingBack fires when the rollback coordinator begins compensating artifacts.
	// In-flight handlers should check the run status and quiesce on receiving this event.
	TopicRunRollingBack = "run.rolling_back"
	// TopicRunRollbackFailed fires when the rollback coordinator encountered a partial
	// failure and the run reached rollback_failed. Operator intervention is required.
	TopicRunRollbackFailed = "run.rollback_failed"
	// TopicRunCancelling fires when an operator-issued cancel transitions a
	// WorkflowRun from in_progress to the cancelling transient state.
	// In-flight subscribers should quiesce their work on behalf of the run.
	TopicRunCancelling = "run.cancelling"
	// TopicRunCancelled fires when the cancellation finalization step
	// transitions a WorkflowRun from cancelling to the cancelled terminal state.
	TopicRunCancelled = "run.cancelled"

	// TopicRunTimeout fires when the watchdog detects a WorkflowRun has been
	// in_progress for longer than its inactivity timeout without any event.
	// This package subscribes and flips the run to failed.
	TopicRunTimeout = "run.timeout"

	// TopicTaskTimeout fires when the watchdog detects a per-step stall
	// (current_step_started_at older than step_timeout).
	TopicTaskTimeout = "task.timeout"
)

// WorkflowRunInProgressPayload is the Publish payload for [TopicRunInProgress].
type WorkflowRunInProgressPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	StartedAt     string `json:"started_at"`
}

// WorkflowRunCompletedPayload is the Publish payload for [TopicRunCompleted].
type WorkflowRunCompletedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	CompletedAt   string `json:"completed_at"`
	DurationMs    int64  `json:"duration_ms,omitempty"`
}

// WorkflowRunFailedPayload is the Publish payload for [TopicRunFailed].
type WorkflowRunFailedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	FailedAt      string `json:"failed_at"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// WorkflowRunRolledBackPayload is the Publish payload for [TopicRunRolledBack].
type WorkflowRunRolledBackPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	RolledBackAt  string `json:"rolled_back_at"`
	Reason        string `json:"reason,omitempty"`
}

// WorkflowRunRollingBackPayload is the Publish payload for [TopicRunRollingBack].
// In-flight handlers that receive this should check the run status and stop
// further work on behalf of this run.
type WorkflowRunRollingBackPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	Reason        string `json:"reason,omitempty"`
}

// WorkflowRunRollbackFailedPayload is the Publish payload for [TopicRunRollbackFailed].
type WorkflowRunRollbackFailedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	FailedAt      string `json:"failed_at"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// WorkflowRunCancellingPayload is the Publish payload for [TopicRunCancelling].
// Subscribers should quiesce in-flight work for the run; the finalization
// step transitions the run to cancelled at or after QuiesceDeadline
// regardless of acknowledgement.
type WorkflowRunCancellingPayload struct {
	WorkflowRunID   string `json:"workflow_run_id"`
	Reason          string `json:"reason,omitempty"`
	CancelledBy     string `json:"cancelled_by,omitempty"`
	QuiesceDeadline string `json:"quiesce_deadline,omitempty"`
}

// WorkflowRunCancelledPayload is the Publish payload for [TopicRunCancelled].
// Marks the run as terminally cancelled.
type WorkflowRunCancelledPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	CancelledAt   string `json:"cancelled_at"`
	Reason        string `json:"reason,omitempty"`
	CancelledBy   string `json:"cancelled_by,omitempty"`
}

// TaskCancelledPayload is the Publish payload for [TopicTaskCancelled].
// Emitted once per Task whose status was flipped to cancelled by the
// run-cancel cascade.
type TaskCancelledPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id"`
	Reason        string `json:"reason,omitempty"`
}

// WorkflowRunTimeoutPayload is the Publish payload for [TopicRunTimeout].
// Published by the watchdog when a WorkflowRun exceeds its inactivity timeout.
type WorkflowRunTimeoutPayload struct {
	WorkflowRunID    string `json:"workflow_run_id"`
	LastEventAt      string `json:"last_event_at,omitempty"`
	InactivityWindow string `json:"inactivity_window,omitempty"`
	DetectedAt       string `json:"detected_at"`
}

// WorkflowRunTaskTimeoutPayload is the Publish payload for [TopicTaskTimeout].
// Published by the watchdog when a per-step stall is detected.
type WorkflowRunTaskTimeoutPayload struct {
	WorkflowRunID        string `json:"workflow_run_id"`
	StepID               string `json:"step_id"`
	CurrentStepStartedAt string `json:"current_step_started_at,omitempty"`
	StepTimeout          string `json:"step_timeout,omitempty"`
	DetectedAt           string `json:"detected_at"`
}

// TaskClassifyFailurePayload is the Publish payload for [TopicTaskClassifyFailure].
// Emitted when a task exhausts its retry budget; an AI classifier responds
// with [TaskFailureClassifiedPayload].
type TaskClassifyFailurePayload struct {
	TaskID                 string `json:"task_id"`
	WorkflowRunID          string `json:"workflow_run_id,omitempty"`
	FailureCount           int    `json:"failure_count"`
	LastFailureReason      string `json:"last_failure_reason,omitempty"`
	TaskDescription        string `json:"task_description,omitempty"`
	FailedTodoTitle        string `json:"failed_todo_title,omitempty"`
	FailedTodoInstructions string `json:"failed_todo_instructions,omitempty"`
}

// TaskFailureClassifiedPayload is the Publish payload for [TopicTaskFailureClassified].
// Emitted by an AI classifier in response to [TopicTaskClassifyFailure].
type TaskFailureClassifiedPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
	// FailureType is "transient" (allow one more retry) or "requires-human" (escalate).
	FailureType string `json:"failure_type"`
	Reasoning   string `json:"reasoning,omitempty"`
}

// FailedCriterionSummary carries the outcome of one failed AcceptanceCriteria
// in a [ReviewOutcomePayload].
type FailedCriterionSummary struct {
	CriterionID string `json:"criterion_id"`
	Title       string `json:"title"`
	Result      string `json:"result"`
	ResultNotes string `json:"result_notes,omitempty"`
}

// ReviewOutcomePayload is the Publish payload for [TopicReviewPassed] and [TopicReviewFailed].
// Emitted by the reviewer after all AcceptanceCriteria have been evaluated.
type ReviewOutcomePayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
	// FailedCriteria lists each criterion that did not pass; empty for review.passed.
	FailedCriteria []FailedCriterionSummary `json:"failed_criteria,omitempty"`
}
