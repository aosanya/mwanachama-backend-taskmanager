package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm/clause"

	"github.com/aosanya/mwanachama-backend-shared/spec"
)

const (
	capInputSetting   = "worker_cap_input_tokens"
	capOutputSetting  = "worker_cap_output_tokens"
	capRuntimeSetting = "worker_cap_runtime_seconds"

	builtInInputCap   = 500000
	builtInOutputCap  = 100000
	builtInRuntimeCap = 900
)

var ErrInvalidCaps = errors.New("a cap is a whole number of at least 1, and a consumption figure is a whole number of at least 0; send at least one cap")

func (m *taskManager) settingNumber(name string, builtIn int64) (int64, error) {
	if _, declared := m.spec.Setting(name); !declared {
		return builtIn, nil
	}
	effective, err := spec.ResolveStored(m.db, m.spec, name)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	switch v := effective.Value.(type) {
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	}
	return 0, fmt.Errorf("%s: %v is not a whole number", name, effective.Value)
}

func (m *taskManager) GetWorkerCapsDefault(ctx context.Context) (WorkerCaps, error) {
	input, err := m.settingNumber(capInputSetting, builtInInputCap)
	if err != nil {
		return WorkerCaps{}, err
	}
	output, err := m.settingNumber(capOutputSetting, builtInOutputCap)
	if err != nil {
		return WorkerCaps{}, err
	}
	runtime, err := m.settingNumber(capRuntimeSetting, builtInRuntimeCap)
	if err != nil {
		return WorkerCaps{}, err
	}
	return WorkerCaps{InputTokens: input, OutputTokens: output, RuntimeSeconds: runtime}, nil
}

func validChange(change TaskCapsChange) bool {
	sent := 0
	for _, n := range []*int64{change.InputTokens, change.OutputTokens, change.RuntimeSeconds} {
		if n == nil {
			continue
		}
		if *n < 1 {
			return false
		}
		sent++
	}
	return sent > 0
}

func (m *taskManager) requireProductOwner(ctx context.Context, requesterID string) error {
	visible, err := m.backlogVisibility(ctx, requesterID)
	if err != nil || visible.all {
		return err
	}
	owner, err := m.storedOwnerGroup(ctx)
	if err != nil {
		return err
	}
	if m.groups != nil && owner != "" {
		held, err := m.groups(ctx, requesterID)
		if err != nil {
			return fmt.Errorf("worker caps: groups: %w", err)
		}
		for _, id := range held {
			if id == owner {
				return nil
			}
		}
	}
	return fmt.Errorf("%w: only the Product owner group sets the default caps", ErrBacklogForbidden)
}

func (m *taskManager) SetWorkerCapsDefault(ctx context.Context, requesterID string, change TaskCapsChange) (WorkerCaps, error) {
	if err := m.requireProductOwner(ctx, requesterID); err != nil {
		return WorkerCaps{}, err
	}
	if !validChange(change) {
		return WorkerCaps{}, ErrInvalidCaps
	}
	for name, n := range map[string]*int64{capInputSetting: change.InputTokens, capOutputSetting: change.OutputTokens, capRuntimeSetting: change.RuntimeSeconds} {
		if n == nil {
			continue
		}
		if err := m.storeSetting(ctx, name, *n); err != nil {
			return WorkerCaps{}, err
		}
	}
	return m.GetWorkerCapsDefault(ctx)
}

func (m *taskManager) upsertRow(ctx context.Context, role string, v any) error {
	row, err := m.store.Row(role, v)
	if err != nil {
		return err
	}
	var changing []string
	for column := range row {
		if column != "id" {
			changing = append(changing, column)
		}
	}
	conflict := clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.AssignmentColumns(changing)}
	return m.store.Query(ctx, role).Clauses(conflict).Create(row).Error
}

func (m *taskManager) ownCaps(ctx context.Context, taskID string) (TaskCapsOverride, bool, error) {
	var own TaskCapsOverride
	err := m.store.Take(m.store.Query(ctx, roleTaskCaps).Where("id = ?", taskID), roleTaskCaps, &own, ErrTaskNotFound)
	if errors.Is(err, ErrTaskNotFound) {
		return TaskCapsOverride{}, false, nil
	}
	return own, err == nil, err
}

func (m *taskManager) usageOf(ctx context.Context, taskID string) (*TaskUsage, error) {
	var used TaskUsage
	err := m.store.Take(m.store.Query(ctx, roleTaskUsage).Where("id = ?", taskID), roleTaskUsage, &used, ErrTaskNotFound)
	if errors.Is(err, ErrTaskNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &used, nil
}

func withOwn(defaults WorkerCaps, own TaskCapsOverride) WorkerCaps {
	if own.InputTokens > 0 {
		defaults.InputTokens = own.InputTokens
	}
	if own.OutputTokens > 0 {
		defaults.OutputTokens = own.OutputTokens
	}
	if own.RuntimeSeconds > 0 {
		defaults.RuntimeSeconds = own.RuntimeSeconds
	}
	return defaults
}

func (m *taskManager) capsReading(ctx context.Context, taskID string) (TaskCaps, error) {
	defaults, err := m.GetWorkerCapsDefault(ctx)
	if err != nil {
		return TaskCaps{}, err
	}
	reading := TaskCaps{TaskID: taskID, Default: defaults, InForce: defaults}
	own, has, err := m.ownCaps(ctx, taskID)
	if err != nil {
		return TaskCaps{}, fmt.Errorf("GetTaskCaps: %w", err)
	}
	if has {
		reading.Own = &WorkerCaps{InputTokens: own.InputTokens, OutputTokens: own.OutputTokens, RuntimeSeconds: own.RuntimeSeconds}
		reading.InForce = withOwn(defaults, own)
	}
	used, err := m.usageOf(ctx, taskID)
	if err == nil {
		reading.Used, err = m.stopIfIdle(ctx, used)
	}
	if err != nil {
		return TaskCaps{}, fmt.Errorf("GetTaskCaps: %w", err)
	}
	return reading, nil
}

func (m *taskManager) GetTaskCaps(ctx context.Context, requesterID, taskID string) (TaskCaps, error) {
	if _, err := m.GetTaskFor(ctx, requesterID, taskID); err != nil {
		return TaskCaps{}, err
	}
	return m.capsReading(ctx, taskID)
}

func (m *taskManager) SetTaskCaps(ctx context.Context, requesterID, taskID string, change TaskCapsChange) (TaskCaps, error) {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return TaskCaps{}, err
	}
	if _, err := m.GetTask(ctx, taskID); err != nil {
		return TaskCaps{}, err
	}
	if !validChange(change) {
		return TaskCaps{}, ErrInvalidCaps
	}
	own, _, err := m.ownCaps(ctx, taskID)
	if err != nil {
		return TaskCaps{}, fmt.Errorf("SetTaskCaps: %w", err)
	}
	own.ID, own.TaskID = taskID, taskID
	if change.InputTokens != nil {
		own.InputTokens = *change.InputTokens
	}
	if change.OutputTokens != nil {
		own.OutputTokens = *change.OutputTokens
	}
	if change.RuntimeSeconds != nil {
		own.RuntimeSeconds = *change.RuntimeSeconds
	}
	if err := m.upsertRow(ctx, roleTaskCaps, own); err != nil {
		return TaskCaps{}, fmt.Errorf("SetTaskCaps: %w", err)
	}
	return m.capsReading(ctx, taskID)
}

func (m *taskManager) ClearTaskCaps(ctx context.Context, requesterID, taskID string) (TaskCaps, error) {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return TaskCaps{}, err
	}
	if _, err := m.GetTask(ctx, taskID); err != nil {
		return TaskCaps{}, err
	}
	if err := m.store.Query(ctx, roleTaskCaps).Where("id = ?", taskID).Delete(nil).Error; err != nil {
		return TaskCaps{}, fmt.Errorf("ClearTaskCaps: %w", err)
	}
	return m.capsReading(ctx, taskID)
}

func (m *taskManager) WriteTaskUsage(ctx context.Context, requesterID, taskID string, usage TaskUsageWrite) (TaskCaps, error) {
	if err := m.requireMoveOnTask(ctx, requesterID, taskID); err != nil {
		return TaskCaps{}, err
	}
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return TaskCaps{}, err
	}
	if usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.SecondsUsed < 0 {
		return TaskCaps{}, ErrInvalidCaps
	}
	previous, err := m.usageOf(ctx, taskID)
	if err != nil {
		return TaskCaps{}, fmt.Errorf("WriteTaskUsage: %w", err)
	}
	row := TaskUsage{
		ID: taskID, TaskID: taskID,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, SecondsUsed: usage.SecondsUsed,
		ClockStopped: usage.ClockStopped, UpdatedAt: time.Now().UTC().Format(noteTimeForm),
	}
	if err := m.upsertRow(ctx, roleTaskUsage, row); err != nil {
		return TaskCaps{}, fmt.Errorf("WriteTaskUsage: %w", err)
	}
	m.logRestart(ctx, taskID, previous, usage)
	reading, err := m.capsReading(ctx, taskID)
	if err != nil {
		return TaskCaps{}, err
	}
	return m.applyCaps(ctx, requesterID, task, usage, reading), nil
}
