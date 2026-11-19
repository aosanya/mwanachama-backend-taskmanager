package mwanachamataskmanager

import (
	"context"
	"fmt"
	"github.com/aosanya/mwanachama-backend-shared/specstore"
)

func (m *taskManager) withLastUpkeep(ctx context.Context, recs []RepeatingTaskRecord) error {
	if len(recs) == 0 {
		return nil
	}
	newest, err := m.newestPeriodPerSchedule(ctx, scheduleIDs(recs))
	if err != nil {
		return err
	}
	doneAt, err := m.completedAtOf(ctx, periodTaskIDs(newest))
	if err != nil {
		return err
	}
	for i := range recs {
		period, ok := newest[recs[i].ID]
		if !ok {
			continue
		}
		recs[i].LastTaskID = period.TaskID
		recs[i].LastDueAt = period.DueAt
		recs[i].LastDoneAt = doneAt[period.TaskID]
	}
	return nil
}
func scheduleIDs(recs []RepeatingTaskRecord) []string {
	ids := make([]string, len(recs))
	for i, rec := range recs {
		ids[i] = rec.ID
	}
	return ids
}
func periodTaskIDs(newest map[string]RepeatingTaskPeriod) []string {
	ids := make([]string, 0, len(newest))
	for _, period := range newest {
		ids = append(ids, period.TaskID)
	}
	return ids
}
func (m *taskManager) newestPeriodPerSchedule(ctx context.Context, scheduleIDs []string) (map[string]RepeatingTaskPeriod, error) {
	q := m.store.Query(ctx, roleRepeatingPeriod).Where("repeating_task_id IN ?", scheduleIDs).Order("period_start DESC").Order("period_key DESC")
	periods, err := specstore.List[RepeatingTaskPeriod](m.store, q, roleRepeatingPeriod)
	if err != nil {
		return nil, fmt.Errorf("last upkeep periods: %w", err)
	}
	newest := make(map[string]RepeatingTaskPeriod, len(scheduleIDs))
	for _, period := range periods {
		if _, seen := newest[period.RepeatingTaskID]; !seen {
			newest[period.RepeatingTaskID] = period
		}
	}
	return newest, nil
}
func (m *taskManager) completedAtOf(ctx context.Context, taskIDs []string) (map[string]string, error) {
	doneAt := make(map[string]string, len(taskIDs))
	if len(taskIDs) == 0 {
		return doneAt, nil
	}
	var rows []struct{ ID, CompletedAt string }
	err := m.store.Query(ctx, roleTask).Where("id IN ?", taskIDs).Select("id", "completed_at").Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("last upkeep completion: %w", err)
	}
	for _, row := range rows {
		doneAt[row.ID] = row.CompletedAt
	}
	return doneAt, nil
}
