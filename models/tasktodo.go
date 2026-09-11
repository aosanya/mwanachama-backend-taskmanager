package models

// TodoStatus represents the lifecycle state of a [TaskTodo].
type TodoStatus string

const (
	// TodoStatusPending is the initial state — waiting to be picked up by an agent.
	TodoStatusPending TodoStatus = "pending"

	// TodoStatusBlocked is a holding state — the todo has unfulfilled
	// depends_on entries. It will not be dispatched until every predecessor
	// todo reaches TodoStatusCompleted.
	TodoStatusBlocked TodoStatus = "blocked"

	// TodoStatusDispatched means an agent has started work on this todo.
	TodoStatusDispatched TodoStatus = "dispatched"

	// TodoStatusCompleted is a terminal state — the agent finished successfully.
	TodoStatusCompleted TodoStatus = "completed"

	// TodoStatusFailed is a terminal state — the agent encountered an error.
	TodoStatusFailed TodoStatus = "failed"

	// TodoStatusSkipped is a terminal state — a direction response chose to
	// bypass this step. Treated as non-failing for parent-task completion.
	TodoStatusSkipped TodoStatus = "skipped"
)

// TaskTodo is a decomposed sub-task produced when a decomposition step
// splits a Task into ordered steps.
type TaskTodo struct {
	ID string `json:"id"`

	// Code is a stable, human-readable identifier (e.g. "TD-1") minted once
	// at creation. It never changes, even when Title is later edited.
	Code string `json:"code"`

	Title        string `json:"title"`
	Description  string `json:"description,omitempty"`
	Instructions string `json:"instructions"`

	// Ordinality is the 1-based position of this todo within the decomposition.
	Ordinality int `json:"ordinality"`

	// CanRunParallel is true when this todo has no predecessor dependency.
	CanRunParallel bool `json:"can_run_parallel"`

	// DependsOn lists the ordinality values of todos that must complete first.
	DependsOn []int `json:"depends_on,omitempty"`

	Status TodoStatus `json:"status"`

	// ParentTaskID is the Task ID from which this todo was decomposed.
	ParentTaskID string `json:"parent_task_id"`

	// DecompRunID is the AgentRun ID that produced this todo.
	DecompRunID string `json:"decomp_run_id,omitempty"`

	// AgentID is the agent assigned to execute this todo.
	AgentID string `json:"agent_id,omitempty"`

	// Precalls is a JSON-encoded []PrecallSpec: pre-execution fetch specs
	// executed before the LLM runs this todo.
	Precalls string `json:"precalls,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	// WorkflowRunID is inherited from the parent Task at creation time.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}
