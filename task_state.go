package mwanachamataskmanager

import "github.com/aosanya/mwanachama-backend-taskmanager/models"

func (m *taskManager) taskStates() []models.TaskState {
	return models.DefaultTaskStates()
}

func (m *taskManager) stateNameFor(status models.TaskStatus) string {
	return models.StateNameFor(m.taskStates(), status)
}
