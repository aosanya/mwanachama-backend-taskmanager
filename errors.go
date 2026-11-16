package mwanachamataskmanager

import "errors"

var ErrTaskNotFound = errors.New("task not found")
var ErrInvalidPlacement = errors.New("invalid card placement")

var ErrInvalidEffort = errors.New("effort is unknown, very_low, low, medium, high or very_high")
var ErrTaskMoveConflict = errors.New("the task changed while it was being moved; read it and move it again")
var ErrPriorityLocked = errors.New("the priority of an issue in Done cannot be changed; move it back to an open step first")

var ErrInvalidOverdueFilter = errors.New("invalid overdue filter")

var ErrInvalidAssignee = errors.New("an assignee is an actor or a role with an id")

var ErrTaskAlreadyExists = errors.New("task already exists")

var ErrInvalidStatusTransition = errors.New("invalid task status transition")

var ErrInvalidTask = errors.New("invalid task: missing required fields")

var ErrAgentNotFound = errors.New("agent not found")

var ErrAgentAlreadyExists = errors.New("agent already exists")

var ErrProjectNotFound = errors.New("project not found")

var ErrTagNotFound = errors.New("tag not found")

var ErrTaskTodoNotFound = errors.New("task todo not found")

var ErrWorkflowRunNotFound = errors.New("workflow run not found")

var ErrInvalidRunStatusTransition = errors.New("invalid workflow run status transition")

var ErrWorkflowRunNameExists = errors.New("workflow run name already exists")

var ErrWorkflowRunMismatch = errors.New("task already belongs to a different workflow run")

var ErrProjectAlreadyExists = errors.New("project already exists")

var ErrInvalidRelationship = errors.New("invalid relationship")

var ErrRelationshipNotFound = errors.New("relationship not found")

var ErrInvalidImport = errors.New("invalid import: markdown document could not be parsed")

var ErrImportJobNotFound = errors.New("import job not found")

var ErrImportJobNotCancellable = errors.New("import job is not cancellable")

var ErrRollbackConflict = errors.New("workflow run rollback already in progress")

var ErrRollbackNotInProgress = errors.New("workflow run is not rolling back")

var ErrForeignRunDependency = errors.New("run closure contains a task depended on by another workflow run")

var ErrFailureBudgetAlreadySet = errors.New("workflow run failure budget already set")

var ErrNotRootWorkflowRun = errors.New("workflow run is not a root run; budget operations must target the root")

var ErrCannotCancelTerminalRun = errors.New("workflow run is not in_progress; cancel rejected")

var ErrDeliverableNotFound = errors.New("deliverable not found")

var ErrAcceptanceCriteriaNotFound = errors.New("acceptance criteria not found")

var ErrBlocked = errors.New("task is blocked by non-terminal blockers")

type BlockedError struct {
	BlockerTaskIDs []string
}

func (e *BlockedError) Error() string {
	return ErrBlocked.Error()
}

func (e *BlockedError) Is(target error) bool {
	return target == ErrBlocked
}
