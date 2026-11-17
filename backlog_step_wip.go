package mwanachamataskmanager

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const hoursInDay = 24

var ErrWipLimit = errors.New("the step is at its limit")

type StepLimitRequest struct {
	BacklogID string `json:"backlog_id"`
	StepID    string `json:"step_id"`
	WipLimit  *int   `json:"wip_limit"`
}

func (m *taskManager) SetBacklogStepWipLimit(ctx context.Context, req StepLimitRequest) (models.BacklogSummary, error) {
	if req.WipLimit == nil || *req.WipLimit < 0 {
		return models.BacklogSummary{}, fmt.Errorf("%w: wip_limit is a whole number, 0 or more; 0 clears it", ErrInvalidBacklog)
	}
	return m.changeBacklog(ctx, req.BacklogID, func(b *models.Backlog) error {
		i, ok := models.FindStep(*b, req.StepID)
		if !ok {
			return fmt.Errorf("%w: %q in %q", ErrUnknownBacklogStep, req.StepID, b.Name)
		}
		b.Steps[i].WipLimit = *req.WipLimit
		return nil
	})
}

func (m *taskManager) SetBacklogStepWipLimitAs(ctx context.Context, requesterID string, req StepLimitRequest) (models.BacklogSummary, error) {
	if err := m.requireEditOn(ctx, requesterID, req.BacklogID); err != nil {
		return models.BacklogSummary{}, err
	}
	return m.SetBacklogStepWipLimit(ctx, req)
}

func (m *taskManager) MoveTaskToStepOverrideAs(ctx context.Context, requesterID, taskID, backlogID, stepID string, override bool) (Task, error) {
	if err := m.requireMoveAccess(ctx, requesterID, taskID, backlogID); err != nil {
		return Task{}, err
	}
	if err := m.checkStepLimit(ctx, requesterID, taskID, backlogID, stepID, override); err != nil {
		return Task{}, err
	}
	return m.MoveTaskToStep(ctx, taskID, backlogID, stepID)
}

func (m *taskManager) checkStepLimit(ctx context.Context, requesterID, taskID, backlogID, stepID string, override bool) error {
	if stepID == "" {
		return nil
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return err
	}
	_, step, err := locateStep(backlogs, backlogID, stepID)
	if err != nil {
		return err
	}
	task, err := m.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	full := m.refuseFullStep(ctx, task, backlogID, step)
	if full == nil || !override {
		return full
	}
	return m.requireEditOn(ctx, requesterID, backlogID)
}

func (m *taskManager) refuseFullStep(ctx context.Context, task Task, backlogID string, step models.BacklogStep) error {
	if step.WipLimit <= 0 || (task.Backlog == backlogID && task.Step == step.ID) {
		return nil
	}
	held, err := m.issuesAtStep(ctx, backlogID, step.ID)
	if err != nil {
		return err
	}
	if held >= step.WipLimit {
		return fmt.Errorf("%w: %q holds %d of its limit of %d issues; finish or move one out, or ask a manager to override", ErrWipLimit, step.Name, held, step.WipLimit)
	}
	return nil
}

func (m *taskManager) issuesAtStep(ctx context.Context, backlogID, stepID string) (int, error) {
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return 0, err
	}
	rows, err := m.taskPlaces(m.db.WithContext(ctx))
	if err != nil {
		return 0, fmt.Errorf("issuesAtStep: %w", err)
	}
	held := 0
	for _, row := range rows {
		if b, s := models.PlaceOf(backlogs, row.task()); b == backlogID && s == stepID {
			held++
		}
	}
	return held, nil
}

func stepClock(task Task, now time.Time) (string, int) {
	for _, candidate := range []string{task.StepEnteredAt, task.UpdatedAt, task.CreatedAt} {
		if candidate == "" {
			continue
		}
		entered, err := time.Parse(time.RFC3339, candidate)
		if err != nil {
			continue
		}
		days := int(now.Sub(entered).Hours() / hoursInDay)
		return candidate, max(days, 0)
	}
	return "", 0
}

func narrowByDaysInStep(q *gorm.DB, f models.OrchestrationFilter, now time.Time) *gorm.DB {
	if f.MinDaysInStep <= 0 {
		return q
	}
	cutoff := now.UTC().Add(-time.Duration(f.MinDaysInStep) * hoursInDay * time.Hour).Format(time.RFC3339)
	return q.Where("COALESCE(NULLIF(step_entered_at, ''), NULLIF(updated_at, ''), created_at) <= ?", cutoff)
}
