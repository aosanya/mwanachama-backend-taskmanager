package mwanachamataskmanager

import (
	"context"
	"fmt"
	"time"
)

const (
	idleStopSetting        = "worker_idle_stop_seconds"
	builtInIdleStopSeconds = 600
	secondsInMinute        = 60
)

func spanWords(seconds int64) string {
	if seconds%secondsInMinute == 0 {
		return fmt.Sprintf("%d minutes", seconds/secondsInMinute)
	}
	return fmt.Sprintf("%d seconds", seconds)
}

func (m *taskManager) stopIfIdle(ctx context.Context, used *TaskUsage) (*TaskUsage, error) {
	if used == nil || used.ClockStopped {
		return used, nil
	}
	limit, err := m.settingNumber(idleStopSetting, builtInIdleStopSeconds)
	if err != nil {
		return nil, err
	}
	written, err := time.Parse(time.RFC3339Nano, used.UpdatedAt)
	if err != nil || time.Since(written) < time.Duration(limit)*time.Second {
		return used, nil
	}
	task, err := m.GetTask(ctx, used.TaskID)
	if err != nil || task.Status != TaskStatusInProgress {
		return used, nil
	}
	stop := m.store.Query(ctx, roleTaskUsage).Where("id = ? AND clock_stopped = ?", used.ID, false).Updates(map[string]any{"clock_stopped": true, "idle_stopped": true})
	if stop.Error != nil {
		return nil, stop.Error
	}
	stopped := *used
	stopped.ClockStopped, stopped.IdleStopped = true, true
	if stop.RowsAffected == 1 {
		m.logAsHub(ctx, used.TaskID, fmt.Sprintf("Clock stopped by the hub: no usage write for %s, so the run is read as interrupted or idle, not as too big. The next usage write restarts it.", spanWords(limit)))
	}
	return &stopped, nil
}

func (m *taskManager) logRestart(ctx context.Context, taskID string, previous *TaskUsage, next TaskUsageWrite) {
	if previous != nil && previous.IdleStopped && !next.ClockStopped {
		m.logAsHub(ctx, taskID, "Clock restarted by a usage write after the hub stopped it as interrupted or idle.")
	}
}
