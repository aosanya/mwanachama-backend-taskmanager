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
	tasks := p.Tasks
	if tasks == nil {
		tasks = []Task{}
	}
	var items json.RawMessage
	var err error
	if len(p.Fields) == 0 {
		items, err = json.Marshal(tasks)
	} else {
		items, err = models.KeepFieldsOfEach(tasks, p.Fields)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Items json.RawMessage `json:"items"`
	}{Items: items})
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
