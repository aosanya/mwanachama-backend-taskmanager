package mwanachamataskmanager

import (
	"context"
	"fmt"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

func (m *taskManager) backlogListCountedFor(ctx context.Context, requesterID string) (models.BacklogList, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return models.BacklogList{}, err
	}
	tasks, err := m.placedTasks(ctx)
	if err != nil {
		return models.BacklogList{}, fmt.Errorf("ListBacklogs: %w", err)
	}
	list := summarize(backlogs, tasks)
	if err := m.countSmartIssues(ctx, requesterID, &list, backlogs); err != nil {
		return models.BacklogList{}, err
	}
	return list, nil
}

func (m *taskManager) countedFor(ctx context.Context, requesterID string, written models.BacklogSummary, writeErr error) (models.BacklogSummary, error) {
	if writeErr != nil {
		return models.BacklogSummary{}, writeErr
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return models.BacklogSummary{}, err
	}
	list := models.BacklogList{Items: []models.BacklogSummary{written}}
	if err := m.countSmartIssues(ctx, requesterID, &list, backlogs); err != nil {
		return models.BacklogSummary{}, err
	}
	return list.Items[0], nil
}

func (m *taskManager) SetBacklogRulesAs(ctx context.Context, requesterID, backlogID string, rules *BacklogRules) (models.BacklogSummary, error) {
	if err := m.requireEditOn(ctx, requesterID, backlogID); err != nil {
		return models.BacklogSummary{}, err
	}
	written, err := m.SetBacklogRules(ctx, backlogID, rules)
	return m.countedFor(ctx, requesterID, written, err)
}
