package mwanachamataskmanager

import (
	"fmt"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func movePath(task Task, step models.BacklogStep) ([]TaskStatus, error) {
	if len(step.Statuses) == 0 || models.StepNamesStatus(step, task.Status) {
		return nil, nil
	}
	reached, ok := StatusPathToState(task.Status, models.TaskState{Name: step.Name, Statuses: step.Statuses})
	if !ok {
		return nil, fmt.Errorf("%w: a task in %s cannot reach %q", ErrNoPathToState, task.Status, step.Name)
	}
	return reached, nil
}
