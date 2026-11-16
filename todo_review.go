package mwanachamataskmanager

import (
	"context"

	"github.com/aosanya/mwanachama-backend-taskmanager/models"
)

const (
	inProgressStepID = "in-progress"
	reviewStepID     = "review"
	everyTodoDone    = "Every todo is Done: moved to Review"
)

func (m *taskManager) moveToReviewWhenAllDone(ctx context.Context, requesterID string, before TaskTodo, after TaskTodo) {
	if before.Status == TodoStatusCompleted || after.Status != TodoStatusCompleted {
		return
	}
	counts, err := m.CountIssueTodos(ctx, []string{after.ParentTaskID})
	if err != nil || counts[after.ParentTaskID].Done != counts[after.ParentTaskID].Total {
		return
	}
	issue, err := m.GetTask(ctx, after.ParentTaskID)
	if err != nil || issue.Step != inProgressStepID {
		return
	}
	backlogs, err := m.backlogs(ctx)
	if err != nil {
		return
	}
	at, ok := models.FindBacklog(backlogs, issue.Backlog)
	if !ok {
		return
	}
	if _, hasReview := models.FindStep(backlogs[at], reviewStepID); !hasReview {
		return
	}
	if _, err := m.MoveTaskToStep(ctx, issue.ID, issue.Backlog, reviewStepID); err != nil {
		return
	}
	m.AddTaskNote(ctx, requesterID, issue.ID, noteByWorker, everyTodoDone, nil)
}
