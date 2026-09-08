package models

// WorkflowRunStatus represents the lifecycle state of a [WorkflowRun].
type WorkflowRunStatus string

const (
	// WorkflowRunStatusPending is the initial state — the run has been
	// created but no Task has yet started executing.
	WorkflowRunStatusPending WorkflowRunStatus = "pending"

	// WorkflowRunStatusInProgress means at least one Task in the run is executing.
	WorkflowRunStatusInProgress WorkflowRunStatus = "in_progress"

	// WorkflowRunStatusCompleted is a terminal state — every Task in the run
	// finished successfully.
	WorkflowRunStatusCompleted WorkflowRunStatus = "completed"

	// WorkflowRunStatusFailed is a terminal state — at least one Task in the
	// run failed.
	WorkflowRunStatusFailed WorkflowRunStatus = "failed"

	// WorkflowRunStatusRolledBack is a terminal state — rollback has
	// compensated all artifacts of this run.
	WorkflowRunStatusRolledBack WorkflowRunStatus = "rolled_back"

	// WorkflowRunStatusRollingBack is a transient state entered when the
	// rollback coordinator begins compensating cross-service artifacts.
	WorkflowRunStatusRollingBack WorkflowRunStatus = "rolling_back"

	// WorkflowRunStatusRollbackFailed is a terminal state indicating the
	// rollback coordinator encountered a partial failure.
	WorkflowRunStatusRollbackFailed WorkflowRunStatus = "rollback_failed"

	// WorkflowRunStatusCancelling is the transient state entered when an
	// operator-issued cancel quiesces in-flight handlers.
	WorkflowRunStatusCancelling WorkflowRunStatus = "cancelling"

	// WorkflowRunStatusCancelled is the terminal state reached after the
	// quiesce deadline elapses.
	WorkflowRunStatusCancelled WorkflowRunStatus = "cancelled"

	// WorkflowRunStatusPaused is a non-terminal hold state entered when at
	// least one Task transitions to awaiting-direction or blocked.
	WorkflowRunStatusPaused WorkflowRunStatus = "paused"
)

// CanTransitionTo reports whether moving from the current status to next is
// a valid state-machine step.
//
//	pending          → in_progress
//	in_progress      → completed, failed, paused, cancelling
//	paused           → in_progress, failed, cancelling
//	cancelling       → cancelled
//	failed/completed/cancelled → rolling_back
//	rolling_back     → rolled_back, rollback_failed
//	rollback_failed  → rolling_back
func (s WorkflowRunStatus) CanTransitionTo(next WorkflowRunStatus) bool {
	switch s {
	case WorkflowRunStatusPending:
		return next == WorkflowRunStatusInProgress
	case WorkflowRunStatusInProgress:
		return next == WorkflowRunStatusCompleted ||
			next == WorkflowRunStatusFailed ||
			next == WorkflowRunStatusPaused ||
			next == WorkflowRunStatusCancelling
	case WorkflowRunStatusPaused:
		return next == WorkflowRunStatusInProgress ||
			next == WorkflowRunStatusFailed ||
			next == WorkflowRunStatusCancelling
	case WorkflowRunStatusCancelling:
		return next == WorkflowRunStatusCancelled
	case WorkflowRunStatusFailed, WorkflowRunStatusCompleted, WorkflowRunStatusCancelled:
		return next == WorkflowRunStatusRollingBack
	case WorkflowRunStatusRollingBack:
		return next == WorkflowRunStatusRolledBack || next == WorkflowRunStatusRollbackFailed
	case WorkflowRunStatusRollbackFailed:
		return next == WorkflowRunStatusRollingBack
	default:
		return false // rolled_back is terminal
	}
}

// IsTerminal reports whether s is a terminal (non-recoverable) status.
// paused is explicitly non-terminal — it resolves to in_progress or failed.
func (s WorkflowRunStatus) IsTerminal() bool {
	switch s {
	case WorkflowRunStatusCompleted, WorkflowRunStatusFailed,
		WorkflowRunStatusRolledBack, WorkflowRunStatusRollbackFailed,
		WorkflowRunStatusCancelled:
		return true
	}
	return false
}

// WorkflowRun anchors the closure of a single orchestrated execution.
// Created by a producer at run start; linked to every Task / TaskTodo it
// produces via their denormalised WorkflowRunID column.
type WorkflowRun struct {
	ID string `json:"id"`

	// Name is a caller-supplied or server-generated human-readable label,
	// globally unique.
	Name string `json:"name"`

	Status WorkflowRunStatus `json:"status"`

	// TriggerEvent names the event that started the run (e.g. "next.requested").
	TriggerEvent string `json:"trigger_event,omitempty"`

	// Initiator is an opaque caller identifier.
	Initiator string `json:"initiator,omitempty"`

	Notes string `json:"notes,omitempty"`

	// AgentRunIDs are AgentRun IDs linked to this run. Opaque strings.
	AgentRunIDs []string `json:"agent_run_ids,omitempty"`

	// FunctionJobIDs are job IDs linked to this run.
	FunctionJobIDs []string `json:"function_job_ids,omitempty"`

	// BranchNames are git branch names linked to this run.
	BranchNames []string `json:"branch_names,omitempty"`

	// TerminalEvent is an optional colon-delimited condition
	// (topic:field=value:field=value) that, when matched by an inbound
	// event, transitions the run from in_progress to completed automatically.
	TerminalEvent string `json:"terminal_event,omitempty"`

	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`

	// ParentWorkflowRunID references the WorkflowRun whose failure spawned
	// this child (recovery) run. Empty on top-level runs.
	ParentWorkflowRunID string `json:"parent_workflow_run_id,omitempty"`

	// RootWorkflowRunID is the top-of-chain run ID — denormalised so the
	// root counter can be read/incremented in O(1).
	RootWorkflowRunID string `json:"root_workflow_run_id,omitempty"`

	// FailurePipelineBudget is the maximum number of recovery-pipeline
	// activations allowed under this run's lineage. Lives only on the root run.
	FailurePipelineBudget int `json:"failure_pipeline_budget,omitempty"`

	// FailurePipelinesUsed counts recovery activations charged to this root run.
	FailurePipelinesUsed int `json:"failure_pipelines_used,omitempty"`

	// CountedChildRunIDs is the dedup set of child run IDs already charged
	// to FailurePipelinesUsed — supports idempotent retries.
	CountedChildRunIDs []string `json:"counted_child_run_ids,omitempty"`

	// CancelledBy is the authenticated identity of the caller who issued the
	// cancel API call.
	CancelledBy string `json:"cancelled_by,omitempty"`

	// CancelReason is the human-readable explanation supplied by the caller
	// when cancelling the run.
	CancelReason string `json:"cancel_reason,omitempty"`

	// CancellingUntil is the RFC 3339 timestamp marking the quiesce deadline.
	CancellingUntil string `json:"cancelling_until,omitempty"`

	// LastEventAt is the RFC 3339 timestamp of the most recent event
	// carrying this run's workflow_run_id. Used by the watchdog sweeper.
	LastEventAt string `json:"last_event_at,omitempty"`

	// TimeoutPublished records that a run-timeout event was already
	// published for this run.
	TimeoutPublished bool `json:"timeout_published,omitempty"`

	// PausedAt, when non-empty, suspends watchdog sweeping for this run.
	PausedAt string `json:"paused_at,omitempty"`

	// CurrentStepID is the plan code of the step currently executing.
	CurrentStepID string `json:"current_step_id,omitempty"`

	// CurrentStepStartedAt is the RFC 3339 timestamp when CurrentStepID was set.
	CurrentStepStartedAt string `json:"current_step_started_at,omitempty"`
}

// WorkflowRunClosure is the full read returned by GetWorkflowRunClosure —
// the run anchor plus every Task / TaskTodo / Relationship in its closure
// and the IDs of foreign (cross-service) entities it referenced.
type WorkflowRunClosure struct {
	Run            WorkflowRun    `json:"run"`
	Tasks          []Task         `json:"tasks"`
	Todos          []TaskTodo     `json:"todos"`
	Edges          []Relationship `json:"edges"`
	AgentRunIDs    []string       `json:"agent_run_ids,omitempty"`
	FunctionJobIDs []string       `json:"function_job_ids,omitempty"`
	BranchNames    []string       `json:"branch_names,omitempty"`
}
