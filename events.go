package mwanachamataskmanager

const (
	TopicTaskCreated = "task.created"

	TopicTaskUpdated = "task.updated"

	TopicTaskStatusChanged = "task.status.changed"

	TopicTaskCompleted = "task.completed"

	TopicTaskFailed = "task.failed"

	TopicTaskAssigned = "task.assigned"

	TopicRelationshipCreated = "relationship.created"

	TopicTodoDispatched = "todo.dispatched"

	TopicTodoCompleted = "todo.completed"

	TopicTaskUpdate = "task.update"

	TopicTaskRolledBack = "task.rolled_back"

	TopicTaskCancelled = "task.cancelled"

	TopicPipelineStarted = "pipeline.started"

	TopicTaskNeedsDirection = "task.needs-direction"

	TopicTaskDirection = "task.direction"

	TopicTaskClassifyFailure = "task.classify-failure"

	TopicTaskFailureClassified = "task.failure-classified"

	TopicRunPaused = "run.paused"

	TopicRunResumed = "run.resumed"

	TopicTaskPlanSplit = "task.request-split"

	TopicReviewPassed = "review.passed"

	TopicReviewFailed = "review.failed"
)

type TaskCreatedPayload struct {
	TaskID   string
	Priority TaskPriority

	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

type TaskUpdatedPayload struct {
	TaskID        string
	ChangedFields []string

	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

type TaskStatusChangedPayload struct {
	TaskID string
	From   TaskStatus
	To     TaskStatus

	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

type TaskCompletedPayload struct {
	TaskID         string
	TerminalStatus TaskStatus

	CompletedAt string

	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

type TaskFailedBy struct {
	AgentID string
}

type TaskFailedPayload struct {
	TaskID   string
	RunID    string
	Reason   string
	FailedBy TaskFailedBy

	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

type TaskRolledBackPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id"`
}

type TaskAssignedPayload struct {
	TaskID      string
	AgentID     string
	RoleName    string
	TaskCode    string
	Title       string
	Description string

	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

type RelationshipCreatedPayload struct {
	FromID string
	ToID   string
	Label  string
}

type TodoDispatchedPayload struct {
	TodoID         string
	TaskID         string
	ParentTaskID   string
	DecompRunID    string
	AgentID        string
	Title          string
	Instructions   string
	Ordinality     int
	CanRunParallel bool
	DependsOn      []string
	Precalls       string
	TodoType       string
	MaxRuns        int

	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

type TodoCompletedPayload struct {
	TodoID       string `json:"todo_id"`
	ParentTaskID string `json:"task_id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	TodoType     string `json:"todo_type"`
	MaxRuns      int    `json:"max_runs,omitempty"`
	RunCount     int    `json:"run_count,omitempty"`

	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

type TaskPlanSplitChildSpec struct {
	TempID      string `json:"temp_id"`
	TaskName    string `json:"task_name,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	RoleName    string `json:"role_name,omitempty"`

	DependsOn []string `json:"depends_on,omitempty"`
}

type TaskPlanSplitPayload struct {
	TaskID        string                   `json:"task_id"`
	WorkflowRunID string                   `json:"workflow_run_id,omitempty"`
	Children      []TaskPlanSplitChildSpec `json:"children"`
}

type TaskUpdatePayload struct {
	TaskID string `json:"task_id"`

	BranchName string `json:"branch_name,omitempty"`
}

const (
	TopicRunInProgress = "run.in_progress"

	TopicRunCompleted = "run.completed"

	TopicRunFailed = "run.failed"

	TopicRunRolledBack = "run.rolled_back"

	TopicRunRollingBack = "run.rolling_back"

	TopicRunRollbackFailed = "run.rollback_failed"

	TopicRunCancelling = "run.cancelling"

	TopicRunCancelled = "run.cancelled"

	TopicRunClosed = "run.closed"

	TopicRunTimeout = "run.timeout"

	TopicTaskTimeout = "task.timeout"
)

type WorkflowRunInProgressPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	StartedAt     string `json:"started_at"`
}

type WorkflowRunCompletedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	CompletedAt   string `json:"completed_at"`
	DurationMs    int64  `json:"duration_ms,omitempty"`
}

type WorkflowRunFailedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	FailedAt      string `json:"failed_at"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type WorkflowRunRolledBackPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	RolledBackAt  string `json:"rolled_back_at"`
	Reason        string `json:"reason,omitempty"`
}

type WorkflowRunRollingBackPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	Reason        string `json:"reason,omitempty"`
}

type WorkflowRunRollbackFailedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	FailedAt      string `json:"failed_at"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type WorkflowRunCancellingPayload struct {
	WorkflowRunID   string `json:"workflow_run_id"`
	Reason          string `json:"reason,omitempty"`
	CancelledBy     string `json:"cancelled_by,omitempty"`
	QuiesceDeadline string `json:"quiesce_deadline,omitempty"`
}

type WorkflowRunCancelledPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	CancelledAt   string `json:"cancelled_at"`
	Reason        string `json:"reason,omitempty"`
	CancelledBy   string `json:"cancelled_by,omitempty"`
}

type WorkflowRunClosedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	ClosedAt      string `json:"closed_at"`
	StepID        string `json:"step_id,omitempty"`
	StepDueAt     string `json:"step_due_at"`
	CancelRunID   string `json:"cancel_run_id"`
}

type TaskCancelledPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id"`
	Reason        string `json:"reason,omitempty"`
}

type WorkflowRunTimeoutPayload struct {
	WorkflowRunID    string `json:"workflow_run_id"`
	LastEventAt      string `json:"last_event_at,omitempty"`
	InactivityWindow string `json:"inactivity_window,omitempty"`
	DetectedAt       string `json:"detected_at"`
}

type WorkflowRunTaskTimeoutPayload struct {
	WorkflowRunID        string `json:"workflow_run_id"`
	StepID               string `json:"step_id"`
	CurrentStepStartedAt string `json:"current_step_started_at,omitempty"`
	StepTimeout          string `json:"step_timeout,omitempty"`
	DetectedAt           string `json:"detected_at"`
}

type TaskClassifyFailurePayload struct {
	TaskID                 string `json:"task_id"`
	WorkflowRunID          string `json:"workflow_run_id,omitempty"`
	FailureCount           int    `json:"failure_count"`
	LastFailureReason      string `json:"last_failure_reason,omitempty"`
	TaskDescription        string `json:"task_description,omitempty"`
	FailedTodoTitle        string `json:"failed_todo_title,omitempty"`
	FailedTodoInstructions string `json:"failed_todo_instructions,omitempty"`
}

type TaskFailureClassifiedPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	FailureType string `json:"failure_type"`
	Reasoning   string `json:"reasoning,omitempty"`
}

type FailedCriterionSummary struct {
	CriterionID string `json:"criterion_id"`
	Title       string `json:"title"`
	Result      string `json:"result"`
	ResultNotes string `json:"result_notes,omitempty"`
}

type ReviewOutcomePayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	FailedCriteria []FailedCriterionSummary `json:"failed_criteria,omitempty"`
}
