package models

type TodoStatus string

const (
	TodoStatusPending    TodoStatus = "pending"
	TodoStatusBlocked    TodoStatus = "blocked"
	TodoStatusDispatched TodoStatus = "dispatched"
	TodoStatusCompleted  TodoStatus = "completed"
	TodoStatusFailed     TodoStatus = "failed"
	TodoStatusSkipped    TodoStatus = "skipped"
)

type TaskTodo struct {
	ID   string `json:"id"`
	Code string `json:"code"`

	Title          string   `json:"title"`
	Description    string   `json:"description,omitempty"`
	Instructions   string   `json:"instructions"`
	Ordinality     int      `json:"ordinality"`
	CanRunParallel bool     `json:"can_run_parallel"`
	DependsOn      []string `json:"depends_on,omitempty"`

	DependsOnOrdinalities []int `json:"depends_on_ordinalities,omitempty" spec:"-"`

	BlockedReason string `json:"blocked_reason,omitempty"`

	Status       TodoStatus `json:"status"`
	ParentTaskID string     `json:"parent_task_id"`
	DecompRunID  string     `json:"decomp_run_id,omitempty"`
	AgentID      string     `json:"agent_id,omitempty"`
	Precalls     string     `json:"precalls,omitempty"`

	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	Deleted bool `json:"deleted"`
}

type TodoCount struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

type TaskTodoPatch struct {
	Title        *string
	Description  *string
	Instructions *string
	DependsOn    *[]string
}
