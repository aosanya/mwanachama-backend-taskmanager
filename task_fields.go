package mwanachamataskmanager

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

type TaskPage struct {
	Tasks  []Task
	Fields []string
}

func (p TaskPage) MarshalJSON() ([]byte, error) {
	if len(p.Fields) == 0 {
		return json.Marshal(p.Tasks)
	}
	kept, err := models.KeepFieldsOfEach(p.Tasks, p.Fields)
	if err != nil {
		return nil, err
	}
	return kept, nil
}

func chooseFields(asked []string, item any) ([]string, error) {
	fields, err := models.ChooseFields(asked, item)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilter, err)
	}
	return fields, nil
}

func (m *taskManager) ListTaskPage(ctx context.Context, filter TaskFilter) (TaskPage, error) {
	fields, err := chooseFields(filter.Fields, Task{})
	if err != nil {
		return TaskPage{}, fmt.Errorf("ListTasks: %w", err)
	}
	tasks, err := m.ListTasks(ctx, filter)
	return TaskPage{Tasks: tasks, Fields: fields}, err
}
