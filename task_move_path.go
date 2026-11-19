package mwanachamataskmanager

import (
	"fmt"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func movePath(task Task, step models.BacklogStep) ([]TaskStatus, error) {
	statuses := step.Statuses
	implied := len(statuses) == 0
	if implied {
		statuses = models.ImpliedStatuses(step, task.Status)
	}
	if len(statuses) == 0 || models.StepNamesStatus(models.BacklogStep{Statuses: statuses}, task.Status) {
		return nil, nil
	}
	if implied && reopening(task.Status, statuses) {
		return statuses[:1], nil
	}
	reached, ok := StatusPathToState(task.Status, models.TaskState{Name: step.Name, Statuses: statuses})
	if !ok {
		return nil, fmt.Errorf("%w: a task in %s cannot reach %q", ErrNoPathToState, task.Status, step.Name)
	}
	return reached, nil
}
func reopening(from TaskStatus, to []TaskStatus) bool {
	closed := from == TaskStatusCompleted || from == TaskStatusCancelled
	return closed && !isTerminalStatus(to[0])
}
