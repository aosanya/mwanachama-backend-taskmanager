// Package models holds mwanachamataskmanager's domain types — pure Go
// structs with zero GORM/DB imports. See gormstore for the row shapes and
// storage mapping.
package models

// TaskStatus represents the lifecycle state of a [Task].
type TaskStatus string

const (
	// TaskStatusPending is the initial state of every new task.
	TaskStatusPending TaskStatus = "pending"

	// TaskStatusInProgress means an agent has claimed and is actively
	// working on the task.
	TaskStatusInProgress TaskStatus = "in_progress"

	// TaskStatusCompleted is a terminal state — the agent finished
	// the task successfully.
	TaskStatusCompleted TaskStatus = "completed"

	// TaskStatusFailed is a terminal state — the agent encountered an
	// unrecoverable error and could not complete the task.
	TaskStatusFailed TaskStatus = "failed"

	// TaskStatusCancelled is a terminal state — the task was abandoned
	// before completion, either by the agent or by an operator.
	TaskStatusCancelled TaskStatus = "cancelled"

	// TaskStatusBlocked means the task has been assigned but is waiting on
	// at least one unfulfilled depends_on edge to a non-terminal source
	// task. Dispatch is held until every dependency reaches a terminal
	// state, at which point the task transitions back to pending and the
	// assignment fires for real.
	TaskStatusBlocked TaskStatus = "blocked"

	// TaskStatusAwaitingDirection is a non-terminal hold state entered when
	// a task has exhausted its automatic retry budget and AI classification
	// determines that human (or AI reviewer) intervention is required.
	TaskStatusAwaitingDirection TaskStatus = "awaiting-direction"

	// TaskStatusSplit marks a task whose planner decided to break it into
	// child Task entities. The parent is inactive; completion is driven by
	// the roll-up of its children.
	TaskStatusSplit TaskStatus = "split"
)

// CanTransitionTo reports whether transitioning from the receiver status to
// next is a valid move in the task lifecycle.
//
//	pending            → in_progress, cancelled, blocked
//	blocked            → pending, cancelled, awaiting-direction
//	in_progress        → completed, failed, cancelled, awaiting-direction, split
//	awaiting-direction → in_progress, blocked, cancelled
//	failed             → in_progress, blocked, cancelled (direction-driven recovery only)
//	split              → completed, failed, cancelled (roll-up from children only)
//	completed          → (none — terminal)
//	cancelled          → (none — terminal)
func (s TaskStatus) CanTransitionTo(next TaskStatus) bool {
	switch s {
	case TaskStatusPending:
		return next == TaskStatusInProgress || next == TaskStatusCancelled || next == TaskStatusBlocked
	case TaskStatusBlocked:
		return next == TaskStatusPending || next == TaskStatusCancelled || next == TaskStatusAwaitingDirection
	case TaskStatusInProgress:
		return next == TaskStatusCompleted || next == TaskStatusFailed ||
			next == TaskStatusCancelled || next == TaskStatusAwaitingDirection ||
			next == TaskStatusSplit
	case TaskStatusAwaitingDirection:
		return next == TaskStatusInProgress || next == TaskStatusBlocked || next == TaskStatusCancelled
	case TaskStatusFailed:
		return next == TaskStatusInProgress || next == TaskStatusBlocked || next == TaskStatusCancelled
	case TaskStatusSplit:
		return next == TaskStatusCompleted || next == TaskStatusFailed || next == TaskStatusCancelled
	default:
		return false
	}
}

// TaskPriority expresses the relative urgency of a [Task].
type TaskPriority string

const (
	TaskPriorityLow      TaskPriority = "low"
	TaskPriorityMedium   TaskPriority = "medium"
	TaskPriorityHigh     TaskPriority = "high"
	TaskPriorityCritical TaskPriority = "critical"
)

// Task is the core domain entity managed by TaskManager.
// All timestamps are ISO 8601 strings (RFC 3339). Empty string means "not set".
type Task struct {
	ID string `json:"id"`

	Description string `json:"description,omitempty"`

	Status TaskStatus `json:"status"`

	Priority TaskPriority `json:"priority"`

	// DueAt is the RFC 3339 deadline; empty string when no deadline is set.
	DueAt string `json:"due_at,omitempty"`

	// Tags are free-form labels associated with the task, resolved from the
	// has_tag join table at read time — not a stored column on Task itself.
	Tags []string `json:"tags,omitempty"`

	// EstimatedHours is the planned effort to complete the task, in hours.
	EstimatedHours float64 `json:"estimated_hours,omitempty"`

	// Context is the AI agent's working memory blob. Optional.
	Context string `json:"context,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// CompletedAt is set when the task reaches a terminal status.
	CompletedAt string `json:"completed_at,omitempty"`

	// Title is the short human-readable label, distinct from Description.
	Title string `json:"title,omitempty"`

	// TaskName is the project-scoped human-readable identifier auto-generated
	// by CreateTaskInProject (e.g. "MVP-001"). Empty for tasks not in a project.
	TaskName string `json:"task_name,omitempty"`

	// ProjectName is the URL-safe slug of the project this task belongs to.
	ProjectName string `json:"project_name,omitempty"`

	SeparateBranch bool   `json:"separate_branch,omitempty"`
	BranchName     string `json:"branch_name,omitempty"`

	// AssignedTo is the ID of the Agent currently responsible for this task.
	// Empty string means unassigned.
	AssignedTo string `json:"assigned_to,omitempty"`

	// WorkflowRunID denormalises the WorkflowRun anchor onto the Task so
	// queries can filter by run-id directly.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	// RecoveryRunsUsed is the number of automatic retry cycles already
	// charged against the task's recovery budget.
	RecoveryRunsUsed int `json:"recovery_runs_used,omitempty"`

	// BlockerNote is the human-readable note supplied by an operator when
	// choosing the mark-blocked direction option.
	BlockerNote string `json:"blocker_note,omitempty"`

	// DirectionHistory is a JSON-encoded []string of past direction options
	// submitted for this task.
	DirectionHistory string `json:"direction_history,omitempty"`

	// ParentTaskID is the ID of the Task that was split to produce this one.
	// Empty for root tasks.
	ParentTaskID string `json:"parent_task_id,omitempty"`
}

// TaskFilter constrains the results returned by TaskManager.ListTasks.
// Zero values mean "no filter" for that field.
type TaskFilter struct {
	Status        TaskStatus
	Priority      TaskPriority
	WorkflowRunID string
}
