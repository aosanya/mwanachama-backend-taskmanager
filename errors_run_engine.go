package mwanachamataskmanager

import "errors"

var ErrWorkflowNotPublished = errors.New("the published agency has no such workflow on that version")

var ErrStepNotInWorkflow = errors.New("that step does not belong to this run's workflow")

var ErrStepOutOfOrder = errors.New("that step does not come next in this run's workflow")

var ErrStepRoleNotHeld = errors.New("the published step names a role the caller does not hold")
