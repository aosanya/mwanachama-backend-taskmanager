package mwanachamataskmanager

import (
	"github.com/aosanya/mwanachama-backend-shared/spec"
	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const taskStatesSetting = "task_states"

func moduleTaskStates(s *spec.Spec) ([]models.TaskState, error) {
	if _, declared := s.Setting(taskStatesSetting); !declared {
		return models.DefaultTaskStates(), nil
	}
	effective, err := spec.Resolve(s.Settings, nil, nil, taskStatesSetting)
	if err != nil {
		return nil, err
	}
	return models.ParseTaskStates(effective.Value)
}

func (m *taskManager) taskStates() []models.TaskState {
	if len(m.states) == 0 {
		return models.DefaultTaskStates()
	}
	return m.states
}

func (m *taskManager) stateNameFor(status models.TaskStatus) string {
	return models.StateNameFor(m.taskStates(), status)
}
