package models

import (
	"encoding/json"
	"fmt"
)

type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"

	TaskStatusInProgress TaskStatus = "in_progress"

	TaskStatusCompleted TaskStatus = "completed"

	TaskStatusFailed TaskStatus = "failed"

	TaskStatusCancelled TaskStatus = "cancelled"

	TaskStatusBlocked TaskStatus = "blocked"

	TaskStatusAwaitingDirection TaskStatus = "awaiting-direction"

	TaskStatusSplit TaskStatus = "split"
)

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

type TaskPriority string

const (
	TaskPriorityLow      TaskPriority = "low"
	TaskPriorityMedium   TaskPriority = "medium"
	TaskPriorityHigh     TaskPriority = "high"
	TaskPriorityCritical TaskPriority = "critical"
)

type TaskEffort string

const (
	TaskEffortUnknown  TaskEffort = "unknown"
	TaskEffortVeryLow  TaskEffort = "very_low"
	TaskEffortLow      TaskEffort = "low"
	TaskEffortMedium   TaskEffort = "medium"
	TaskEffortHigh     TaskEffort = "high"
	TaskEffortVeryHigh TaskEffort = "very_high"
)

type Task struct {
	ID string `json:"id"`

	Description string `json:"description,omitempty"`

	Status TaskStatus `json:"status"`

	State string `json:"state,omitempty" spec:"-"`

	Priority TaskPriority `json:"priority"`

	Effort TaskEffort `json:"effort"`

	DueAt string `json:"due_at,omitempty"`

	Tags []string `json:"tags,omitempty"`

	EstimatedHours float64 `json:"estimated_hours,omitempty"`

	Context string `json:"context,omitempty"`

	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`

	CompletedAt string `json:"completed_at,omitempty"`

	Title string `json:"title,omitempty"`

	TaskName string `json:"task_name,omitempty"`

	ProjectName string `json:"project_name,omitempty"`

	SeparateBranch bool   `json:"separate_branch,omitempty"`
	BranchName     string `json:"branch_name,omitempty"`

	AssignedTo string `json:"assigned_to,omitempty"`

	AssigneeKind AssigneeKind `json:"assignee_kind,omitempty"`

	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	RecoveryRunsUsed int `json:"recovery_runs_used,omitempty"`

	BlockerNote string `json:"blocker_note,omitempty"`

	DirectionHistory string `json:"direction_history,omitempty"`

	ParentTaskID string `json:"parent_task_id,omitempty"`

	HeldBy string `json:"held_by,omitempty"`

	Code       string `json:"code"`
	CodeSeries string `json:"code_series"`
	CodeNumber int    `json:"code_number"`

	LinkedRef string `json:"linked_ref,omitempty"`

	Backlog string `json:"backlog,omitempty"`
	Step    string `json:"step,omitempty"`

	Rank int `json:"rank"`

	Deleted bool `json:"deleted"`
}

type TagList []string

func (l *TagList) UnmarshalJSON(raw []byte) error {
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		*l = many
		return nil
	}
	var one string
	if err := json.Unmarshal(raw, &one); err != nil {
		return fmt.Errorf("tag must be a tag name or a list of tag names")
	}
	*l = TagList{one}
	return nil
}

type TaskFilter struct {
	Status        TaskStatus   `query:"status"`
	Priority      TaskPriority `query:"priority"`
	WorkflowRunID string       `query:"workflow_run_id"`
	HeldBy        string       `query:"held_by"`
	Tag           TagList      `query:"tag"`
	Repo          string       `query:"repo"`
	Backlog       string       `query:"backlog"`
	Step          string       `query:"step"`
	UntouchedDays int          `query:"untouched_days"`
	Offset        int          `query:"offset,min=0"`
	RequesterID   string       `query:"-" json:"-"`

	Limit int `query:"limit,min=1"`
}
