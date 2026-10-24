package mwanachamataskmanager

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-shared/spec"
	"gorm.io/gorm"

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

func StoredTaskStates(db *gorm.DB, s *spec.Spec) ([]models.TaskState, error) {
	if _, declared := s.Setting(taskStatesSetting); !declared {
		return models.DefaultTaskStates(), nil
	}
	effective, err := spec.ResolveStored(db, s, taskStatesSetting)
	if err != nil {
		return nil, err
	}
	return models.ParseTaskStates(effective.Value)
}

func DefineTaskStates(db *gorm.DB, s *spec.Spec, value any) error {
	if _, err := models.ParseTaskStates(value); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		definition, _, err := spec.LoadSettingLayers(tx, s)
		if err != nil {
			return err
		}
		definition[taskStatesSetting] = value
		return spec.PutDefinitionSettings(tx, s, definition)
	})
}

func OverrideTaskStates(db *gorm.DB, s *spec.Spec, value any, by, at string) (spec.SettingChange, error) {
	if _, err := models.ParseTaskStates(value); err != nil {
		return spec.SettingChange{}, err
	}
	return spec.StoreOverride(db, s, taskStatesSetting, value, by, at)
}

func ClearTaskStatesOverride(db *gorm.DB, s *spec.Spec, by, at string) (spec.SettingChange, bool, error) {
	return spec.StoreClearOverride(db, s, taskStatesSetting, by, at)
}

func (m *taskManager) taskStates(ctx context.Context) ([]models.TaskState, error) {
	if m == nil || m.spec == nil {
		return models.DefaultTaskStates(), nil
	}
	states, err := StoredTaskStates(m.db.WithContext(ctx), m.spec)
	if err != nil {
		return nil, fmt.Errorf("task states: %w", err)
	}
	return states, nil
}
