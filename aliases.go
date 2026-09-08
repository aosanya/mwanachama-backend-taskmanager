package mwanachamataskmanager

import "github.com/aosanya/mwanachama-backend-taskmanager/models"

// Root-level aliases so callers need only this package's import, not
// models's directly — see doc.go and the org's entitygraph-to-GORM
// migration convention (established by orgsettings/orgpolicy).
type (
	Task               = models.Task
	TaskStatus         = models.TaskStatus
	TaskPriority       = models.TaskPriority
	TaskFilter         = models.TaskFilter
	TaskTodo           = models.TaskTodo
	TodoStatus         = models.TodoStatus
	Agent              = models.Agent
	Project            = models.Project
	Tag                = models.Tag
	Deliverable        = models.Deliverable
	AcceptanceCriteria = models.AcceptanceCriteria
	WorkflowRun        = models.WorkflowRun
	WorkflowRunStatus  = models.WorkflowRunStatus
	WorkflowRunClosure = models.WorkflowRunClosure
	ImportResult       = models.ImportResult
	ImportProjectJob   = models.ImportProjectJob
	Relationship       = models.Relationship
	Direction          = models.Direction
)

const (
	TaskStatusPending           = models.TaskStatusPending
	TaskStatusInProgress        = models.TaskStatusInProgress
	TaskStatusCompleted         = models.TaskStatusCompleted
	TaskStatusFailed            = models.TaskStatusFailed
	TaskStatusCancelled         = models.TaskStatusCancelled
	TaskStatusBlocked           = models.TaskStatusBlocked
	TaskStatusAwaitingDirection = models.TaskStatusAwaitingDirection
	TaskStatusSplit             = models.TaskStatusSplit

	TaskPriorityLow      = models.TaskPriorityLow
	TaskPriorityMedium   = models.TaskPriorityMedium
	TaskPriorityHigh     = models.TaskPriorityHigh
	TaskPriorityCritical = models.TaskPriorityCritical

	TodoStatusPending    = models.TodoStatusPending
	TodoStatusBlocked    = models.TodoStatusBlocked
	TodoStatusDispatched = models.TodoStatusDispatched
	TodoStatusCompleted  = models.TodoStatusCompleted
	TodoStatusFailed     = models.TodoStatusFailed
	TodoStatusSkipped    = models.TodoStatusSkipped

	WorkflowRunStatusPending        = models.WorkflowRunStatusPending
	WorkflowRunStatusInProgress     = models.WorkflowRunStatusInProgress
	WorkflowRunStatusCompleted      = models.WorkflowRunStatusCompleted
	WorkflowRunStatusFailed         = models.WorkflowRunStatusFailed
	WorkflowRunStatusRolledBack     = models.WorkflowRunStatusRolledBack
	WorkflowRunStatusRollingBack    = models.WorkflowRunStatusRollingBack
	WorkflowRunStatusRollbackFailed = models.WorkflowRunStatusRollbackFailed
	WorkflowRunStatusCancelling     = models.WorkflowRunStatusCancelling
	WorkflowRunStatusCancelled      = models.WorkflowRunStatusCancelled
	WorkflowRunStatusPaused         = models.WorkflowRunStatusPaused

	DirectionInbound  = models.DirectionInbound
	DirectionOutbound = models.DirectionOutbound

	RelLabelAssignedTo            = models.RelLabelAssignedTo
	RelLabelBlocks                = models.RelLabelBlocks
	RelLabelSubtaskOf             = models.RelLabelSubtaskOf
	RelLabelDependsOn             = models.RelLabelDependsOn
	RelLabelMemberOf              = models.RelLabelMemberOf
	RelLabelHasTag                = models.RelLabelHasTag
	RelLabelHasTodo               = models.RelLabelHasTodo
	RelLabelTodoAssignedTo        = models.RelLabelTodoAssignedTo
	RelLabelStartedTask           = models.RelLabelStartedTask
	RelLabelStartedTodo           = models.RelLabelStartedTodo
	RelLabelPartOfRun             = models.RelLabelPartOfRun
	RelLabelHasDeliverable        = models.RelLabelHasDeliverable
	RelLabelHasAcceptanceCriteria = models.RelLabelHasAcceptanceCriteria
)
